package relay

import "io"

type Ledger struct {
	writer     io.Writer
	prevSigRaw []byte
	records    chan []byte
}

func NewLedger(w io.Writer) *Ledger {
	l := &Ledger{writer: w}
	_ = l.writeHeader()
	go l.loop()
	return l
}

func (l *Ledger) writeHeader() error {
	l.prevSigRaw = l.sign(nil)
	_, err := l.writer.Write(nil)
	return err
}

func (l *Ledger) loop() {
	for rec := range l.records {
		sig := l.sign(rec)
		_, _ = l.writer.Write(rec)
		l.prevSigRaw = sig
	}
}

func (l *Ledger) sign(b []byte) []byte { return b }

// flush is a new helper that writes the ledger itself.
func (l *Ledger) flush(rec []byte) {
	_, _ = l.writer.Write(rec) // want `flush writes the ledger directly`
	l.prevSigRaw = l.sign(rec) // want `flush advances the ledger chain` `flush signs a ledger record`
}

func Checkpoint(l *Ledger, rec []byte) {
	go func() {
		_, _ = l.writer.Write(rec) // want `Checkpoint writes the ledger directly`
	}()
	l.records <- rec
}

// other has a writer too, and is not the ledger.
type other struct{ writer io.Writer }

func (o *other) Write(b []byte) { _, _ = o.writer.Write(b) }
