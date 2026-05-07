package relay

import (
	"crypto/rand"
	"net/http"
	"testing"
)

// Real PyPI packages for end-to-end download benchmarks.
var testPackages = []struct {
	name string
	url  string
}{
	{"requests-65KB", "https://files.pythonhosted.org/packages/d7/8e/7540e8a2036f79a125c1d2ebadf69ed7901608859186c856fa0388ef4197/requests-2.33.1-py3-none-any.whl"},
	{"numpy-16MB", "https://files.pythonhosted.org/packages/68/62/63417c13aa35d57bee1337c67446761dc25ea6543130cf868eace6e8157b/numpy-2.4.4-cp311-cp311-manylinux_2_27_aarch64.manylinux_2_28_aarch64.whl"},
	{"pytorch-530MB", "https://files.pythonhosted.org/packages/f9/1e/18a9b10b4bd34f12d4e561c52b0ae7158707b8193c6cfc0aad2b48167090/torch-2.11.0-cp310-cp310-manylinux_2_28_x86_64.whl"},
}

// BenchmarkE2EDownload downloads real packages through the production
// parallel hashing path (identical to what the relay does for response bodies).
func BenchmarkE2EDownload(b *testing.B) {
	for _, pkg := range testPackages {
		b.Run(pkg.name, func(b *testing.B) {
			for b.Loop() {
				resp, err := http.Get(pkg.url)
				if err != nil {
					b.Skipf("network unavailable: %v", err)
					return
				}
				hs := newHasherSet(defaultHashes)
				buf := make([]byte, 32*1024)
				var size int64
				for {
					n, err := resp.Body.Read(buf)
					if n > 0 {
						hs.write(buf[:n])
						size += int64(n)
					}
					if err != nil {
						break
					}
				}
				resp.Body.Close()
				hs.sums()
				b.ReportMetric(float64(size), "bytes")
			}
		})
	}
}

// BenchmarkE2ELocalStreaming measures pure hashing throughput at various sizes
// using 32KB chunks (simulating network read patterns) without network variability.
func BenchmarkE2ELocalStreaming(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"65KB", 65 * 1024},
		{"16MB", 16 * 1024 * 1024},
		{"128MB", 128 * 1024 * 1024},
	}
	for _, s := range sizes {
		data := make([]byte, s.size)
		rand.Read(data)
		b.Run(s.name, func(b *testing.B) {
			for b.Loop() {
				hs := newHasherSet(defaultHashes)
				for off := 0; off < len(data); off += 32 * 1024 {
					end := off + 32*1024
					if end > len(data) {
						end = len(data)
					}
					hs.write(data[off:end])
				}
				hs.sums()
			}
		})
	}
}
