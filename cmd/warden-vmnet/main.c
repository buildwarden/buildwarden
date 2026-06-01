/*
 * warden-vmnet: Privileged helper that creates a vmnet interface and bridges
 * Ethernet frames between it and a SOCK_DGRAM socketpair FD.
 *
 * This binary must run as root (or with com.apple.vm.networking entitlement).
 * It creates a vmnet host-mode interface with isolation, then proxies frames
 * bidirectionally between the vmnet interface and the socketpair.
 *
 * The socketpair's other end is given to VZFileHandleNetworkDeviceAttachment
 * by the orchestrator process. vmnet provides the carrier signaling that
 * macOS guests require to bring up their virtio-net interface.
 *
 * Usage: warden-vmnet --fd=N [--subnet=10.0.0.0/30]
 *
 * The helper writes "ready\n" to stdout once the vmnet interface is up,
 * then bridges frames until the socketpair is closed or SIGTERM is received.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <signal.h>
#include <sys/socket.h>
#include <sys/select.h>
#include <arpa/inet.h>
#include <uuid/uuid.h>
#include <vmnet/vmnet.h>

#define MAX_PKT_SIZE 1600
#define MAX_PKTS 32

static volatile sig_atomic_t g_running = 1;

static void handle_signal(int sig) {
    (void)sig;
    g_running = 0;
}

static void usage(void) {
    fprintf(stderr, "Usage: warden-vmnet --fd=N [--subnet=A.B.C.D/M]\n");
    exit(1);
}

int main(int argc, char **argv) {
    int sock_fd = -1;
    const char *subnet_str = "10.0.0.0/30";

    for (int i = 1; i < argc; i++) {
        if (strncmp(argv[i], "--fd=", 5) == 0) {
            sock_fd = atoi(argv[i] + 5);
        } else if (strncmp(argv[i], "--subnet=", 9) == 0) {
            subnet_str = argv[i] + 9;
        } else {
            usage();
        }
    }

    if (sock_fd < 0) {
        fprintf(stderr, "error: --fd is required\n");
        usage();
    }

    /* Parse subnet: extract gateway IP and mask */
    char subnet_copy[64];
    strncpy(subnet_copy, subnet_str, sizeof(subnet_copy) - 1);
    char *slash = strchr(subnet_copy, '/');
    if (!slash) {
        fprintf(stderr, "error: subnet must be CIDR (e.g., 10.0.0.0/30)\n");
        return 1;
    }
    *slash = '\0';
    int prefix_len = atoi(slash + 1);

    struct in_addr base_addr;
    if (inet_pton(AF_INET, subnet_copy, &base_addr) != 1) {
        fprintf(stderr, "error: invalid subnet address\n");
        return 1;
    }

    /* Gateway = base + 1 */
    uint32_t base_host = ntohl(base_addr.s_addr);
    uint32_t gw_host = base_host + 1;
    uint32_t end_host = base_host + 2;
    uint32_t mask = (prefix_len == 32) ? 0xFFFFFFFF
                                       : ~((1U << (32 - prefix_len)) - 1);

    struct in_addr gw_addr, end_addr, mask_addr;
    gw_addr.s_addr = htonl(gw_host);
    end_addr.s_addr = htonl(end_host);
    mask_addr.s_addr = htonl(mask);

    char gw_str[INET_ADDRSTRLEN], end_str[INET_ADDRSTRLEN], mask_str[INET_ADDRSTRLEN];
    inet_ntop(AF_INET, &gw_addr, gw_str, sizeof(gw_str));
    inet_ntop(AF_INET, &end_addr, end_str, sizeof(end_str));
    inet_ntop(AF_INET, &mask_addr, mask_str, sizeof(mask_str));

    signal(SIGTERM, handle_signal);
    signal(SIGINT, handle_signal);

    /* Create vmnet interface in host mode with isolation */
    xpc_object_t desc = xpc_dictionary_create(NULL, NULL, 0);
    xpc_dictionary_set_uint64(desc, vmnet_operation_mode_key, VMNET_HOST_MODE);

    /* Use a unique network identifier for isolation */
    uuid_t net_uuid;
    uuid_generate(net_uuid);
    xpc_dictionary_set_uuid(desc, vmnet_network_identifier_key, net_uuid);
    xpc_dictionary_set_bool(desc, vmnet_enable_isolation_key, true);

    /* Set host IP on the vmnet interface */
    xpc_dictionary_set_string(desc, vmnet_host_ip_address_key, gw_str);
    xpc_dictionary_set_string(desc, vmnet_host_subnet_mask_key, mask_str);

    fprintf(stderr, "warden-vmnet: creating host-mode interface "
            "(gw=%s mask=%s)\n", gw_str, mask_str);

    __block interface_ref iface = NULL;
    __block vmnet_return_t iface_status = VMNET_FAILURE;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    dispatch_queue_t vmnet_q = dispatch_queue_create(
        "com.buildwarden.vmnet", DISPATCH_QUEUE_SERIAL);

    iface = vmnet_start_interface(desc, vmnet_q,
        ^(vmnet_return_t status, xpc_object_t _Nullable params) {
            iface_status = status;
            if (status == VMNET_SUCCESS && params) {
                const char *mac = xpc_dictionary_get_string(params,
                    vmnet_mac_address_key);
                uint64_t mtu = xpc_dictionary_get_uint64(params,
                    vmnet_mtu_key);
                fprintf(stderr, "warden-vmnet: interface up "
                        "(mac=%s mtu=%llu)\n", mac ? mac : "?", mtu);
            } else {
                fprintf(stderr, "warden-vmnet: start failed "
                        "(status=%u)\n", status);
            }
            dispatch_semaphore_signal(sem);
        });

    if (iface == NULL) {
        fprintf(stderr, "warden-vmnet: vmnet_start_interface returned NULL\n");
        return 1;
    }

    dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
    if (iface_status != VMNET_SUCCESS) {
        fprintf(stderr, "warden-vmnet: interface start failed (%u)\n",
                iface_status);
        return 1;
    }

    /* Set up event callback for packets available */
    int pipe_fds[2];
    if (pipe(pipe_fds) != 0) {
        perror("pipe");
        return 1;
    }
    int notify_read_fd = pipe_fds[0];
    int notify_write_fd = pipe_fds[1];

    vmnet_interface_set_event_callback(iface,
        VMNET_INTERFACE_PACKETS_AVAILABLE, vmnet_q,
        ^(interface_event_t mask __unused, xpc_object_t event __unused) {
            char c = 1;
            write(notify_write_fd, &c, 1);
        });

    /* Signal readiness to the orchestrator */
    printf("ready\n");
    fflush(stdout);

    fprintf(stderr, "warden-vmnet: bridging fd=%d <-> vmnet\n", sock_fd);

    /* Set socket buffer sizes */
    int sndbuf = 1048576;  /* 1 MiB */
    int rcvbuf = 4194304;  /* 4 MiB */
    setsockopt(sock_fd, SOL_SOCKET, SO_SNDBUF, &sndbuf, sizeof(sndbuf));
    setsockopt(sock_fd, SOL_SOCKET, SO_RCVBUF, &rcvbuf, sizeof(rcvbuf));

    /* Bridge loop */
    uint8_t pkt_buf[MAX_PKTS][MAX_PKT_SIZE];
    struct iovec iov[MAX_PKTS];
    struct vmpktdesc pkts[MAX_PKTS];

    for (int i = 0; i < MAX_PKTS; i++) {
        iov[i].iov_base = pkt_buf[i];
        iov[i].iov_len = MAX_PKT_SIZE;
        pkts[i].vm_pkt_iov = &iov[i];
        pkts[i].vm_pkt_iovcnt = 1;
        pkts[i].vm_flags = 0;
    }

    while (g_running) {
        fd_set rfds;
        FD_ZERO(&rfds);
        FD_SET(sock_fd, &rfds);
        FD_SET(notify_read_fd, &rfds);

        int maxfd = (sock_fd > notify_read_fd) ? sock_fd : notify_read_fd;

        struct timeval tv = { .tv_sec = 1, .tv_usec = 0 };
        int ret = select(maxfd + 1, &rfds, NULL, NULL, &tv);
        if (ret < 0) {
            if (g_running) perror("select");
            break;
        }
        if (ret == 0) continue;

        /* VM → vmnet: read from socketpair, write to vmnet */
        if (FD_ISSET(sock_fd, &rfds)) {
            uint8_t frame[MAX_PKT_SIZE];
            ssize_t n = recv(sock_fd, frame, sizeof(frame), 0);
            if (n <= 0) {
                fprintf(stderr, "warden-vmnet: socketpair closed\n");
                break;
            }

            struct iovec wiov = { .iov_base = frame, .iov_len = (size_t)n };
            struct vmpktdesc wpkt = {
                .vm_pkt_size = (size_t)n,
                .vm_pkt_iov = &wiov,
                .vm_pkt_iovcnt = 1,
                .vm_flags = 0,
            };
            int cnt = 1;
            vmnet_return_t wr = vmnet_write(iface, &wpkt, &cnt);
            if (wr != VMNET_SUCCESS) {
                fprintf(stderr, "warden-vmnet: vmnet_write failed (%u)\n", wr);
            }
        }

        /* vmnet → VM: read from vmnet, write to socketpair */
        if (FD_ISSET(notify_read_fd, &rfds)) {
            /* Drain notification pipe */
            char drain[64];
            read(notify_read_fd, drain, sizeof(drain));

            /* Read all available packets */
            for (;;) {
                int pktcnt = MAX_PKTS;
                for (int i = 0; i < MAX_PKTS; i++) {
                    iov[i].iov_len = MAX_PKT_SIZE;
                    pkts[i].vm_pkt_size = MAX_PKT_SIZE;
                }

                vmnet_return_t rr = vmnet_read(iface, pkts, &pktcnt);
                if (rr != VMNET_SUCCESS || pktcnt == 0) break;

                for (int i = 0; i < pktcnt; i++) {
                    send(sock_fd, iov[i].iov_base, pkts[i].vm_pkt_size, 0);
                }
            }
        }
    }

    fprintf(stderr, "warden-vmnet: shutting down\n");
    vmnet_interface_set_event_callback(iface,
        VMNET_INTERFACE_PACKETS_AVAILABLE, NULL, NULL);

    dispatch_semaphore_t stop_sem = dispatch_semaphore_create(0);
    vmnet_stop_interface(iface, vmnet_q, ^(vmnet_return_t status) {
        (void)status;
        dispatch_semaphore_signal(stop_sem);
    });
    dispatch_semaphore_wait(stop_sem, dispatch_time(DISPATCH_TIME_NOW,
        5 * NSEC_PER_SEC));

    close(sock_fd);
    close(notify_read_fd);
    close(notify_write_fd);

    return 0;
}
