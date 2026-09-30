//go:build windows

package hyperv

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName        = "WardenHyperV"
	serviceDisplayName = "BuildWarden Hyper-V privileged helper"
	serviceDescription = "Performs Hyper-V host network standup (New-VMSwitch/New-NetNat/New-NetIPAddress) on behalf of non-elevated BuildWarden builds, over an ACL'd named pipe. Only the typed network-standup operations are exposed."

	// pipeSDDL restricts the pipe to LocalSystem (SY), Built-in Administrators
	// (BA) and Hyper-V Administrators (well-known SID S-1-5-32-578). GA = generic
	// all; leading D:P makes the DACL protected (no inherited ACEs). No other
	// principal can open the pipe, so only a privileged caller can ask the
	// service to stand up a network — and even then only via the typed op set.
	pipeSDDL = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;S-1-5-32-578)"
)

// dialServicePipe connects to the privileged service pipe with a bounded wait.
func dialServicePipe(timeout time.Duration) (net.Conn, error) {
	return winio.DialPipe(ServicePipeName, &timeout)
}

// serviceReachable pings the pipe (short timeout). Used by capability detection
// to set ServiceReachable without side effects.
func serviceReachable() bool {
	conn, err := dialServicePipe(750 * time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, err = clientRoundTrip(conn, svcRequest{Op: opPing})
	return err == nil
}

// runPipeServer listens on the ACL'd pipe and serves each connection until ctx
// is cancelled. It hosts a localProvisioner — the single source of privileged
// logic — and the service runs as LocalSystem, so that provisioner holds full
// Administrator.
func runPipeServer(ctx context.Context) error {
	l, err := winio.ListenPipe(ServicePipeName, &winio.PipeConfig{SecurityDescriptor: pipeSDDL})
	if err != nil {
		return fmt.Errorf("listen pipe %s: %w", ServicePipeName, err)
	}
	defer l.Close()

	prov := &localProvisioner{}
	go func() {
		<-ctx.Done()
		_ = l.Close() // unblocks Accept
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("pipe accept: %w", err)
			}
		}
		go func() {
			defer conn.Close()
			// Network standup is bounded; cap the connection so a wedged caller
			// can't hold the handler forever.
			_ = conn.SetDeadline(time.Now().Add(5 * time.Minute))
			_ = serveConn(ctx, conn, prov)
		}()
	}
}

// svcHandler adapts runPipeServer to the Windows SCM lifecycle.
type svcHandler struct{}

func (svcHandler) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan error, 1)
	go func() { errc <- runPipeServer(ctx) }()

	const accepts = svc.AcceptStop | svc.AcceptShutdown
	s <- svc.Status{State: svc.Running, Accepts: accepts}
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				s <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s <- svc.Status{State: svc.StopPending}
				cancel()
				<-errc
				return false, 0
			}
		case <-errc:
			// Server exited on its own (listen error): report a service error.
			return false, 1
		}
	}
}

// RunService runs the privileged helper. Started by the SCM it drives the
// service lifecycle; run from a console (`warden hyperv service run`) it serves
// interactively until ctx is cancelled (Ctrl-C), which is how it is smoke-tested
// before installing.
func RunService(ctx context.Context) error {
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("detect service context: %w", err)
	}
	if isSvc {
		return svc.Run(serviceName, svcHandler{})
	}
	fmt.Fprintf(os.Stderr, "warden hyperv service: serving on %s (Ctrl-C to stop)\n", ServicePipeName)
	return runPipeServer(ctx)
}

// InstallService registers the LocalSystem service pointing at this executable
// with `hyperv service run`, sets automatic start, and starts it. Requires full
// Administrator (connecting to the SCM with create rights).
func InstallService() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager (run from an elevated shell): %w", err)
	}
	defer m.Disconnect()

	if existing, err := m.OpenService(serviceName); err == nil {
		existing.Close()
		return fmt.Errorf("service %q is already installed; run `warden hyperv service uninstall` first", serviceName)
	}
	s, err := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
		StartType:   mgr.StartAutomatic,
		// Account defaults to LocalSystem, which holds full Administrator — the
		// privilege the service exists to lend to non-elevated builds.
	}, "hyperv", "service", "run")
	if err != nil {
		return fmt.Errorf("create service (run from an elevated shell): %w", err)
	}
	defer s.Close()
	if err := s.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	return nil
}

// UninstallService stops and deletes the service. Requires Administrator.
func UninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager (run from an elevated shell): %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %q is not installed", serviceName)
	}
	defer s.Close()
	_, _ = s.Control(svc.Stop) // best-effort stop before delete
	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}
