# Logging DNS forwarder for the vz [vm] dns loopback case: 127.0.0.1:53 -> 1.1.1.1:53, UDP and TCP.
import socket, socketserver, struct, sys, threading, time
UP = ("1.1.1.1", 53)
LOG = open(sys.argv[1], "a", buffering=1)
def qname(b):
    i, parts = 12, []
    while i < len(b) and b[i]:
        n = b[i]; parts.append(b[i+1:i+1+n].decode(errors="replace")); i += n + 1
    return ".".join(parts)
def log(proto, data): LOG.write(f"{time.strftime('%H:%M:%S')} {proto} {qname(data)}\n")
class U(socketserver.BaseRequestHandler):
    def handle(self):
        data, sock = self.request; log("udp", data)
        u = socket.socket(socket.AF_INET, socket.SOCK_DGRAM); u.settimeout(5)
        try: u.sendto(data, UP); sock.sendto(u.recv(65535), self.client_address)
        except OSError as e: LOG.write(f"udp error {e}\n")
class T(socketserver.BaseRequestHandler):
    def handle(self):
        h = self.request.recv(2)
        if len(h) < 2: return
        q = self.request.recv(struct.unpack("!H", h)[0]); log("tcp", q)
        with socket.create_connection(UP, 5) as t:
            t.sendall(h + q); self.request.sendall(t.recv(65535))
class TU(socketserver.ThreadingMixIn, socketserver.UDPServer): allow_reuse_address = True
class TT(socketserver.ThreadingMixIn, socketserver.TCPServer): allow_reuse_address = True
us, ts = TU(("127.0.0.1", 53), U), TT(("127.0.0.1", 53), T)
LOG.write("listening 127.0.0.1:53\n")
threading.Thread(target=ts.serve_forever, daemon=True).start(); us.serve_forever()
