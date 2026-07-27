package recorder

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/foxglove/mcap/go/mcap"
)

// RecoverPartials finalizes episodes orphaned by a crash or power cut. A
// footerless partial is not a valid standalone MCAP, so re-lex the records and
// rewrite a properly-footered file, then mark the sidecar crashed with an
// unknown outcome.
func RecoverPartials(dir string) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".mcap.partial") {
			continue
		}
		id := strings.TrimSuffix(name, ".mcap.partial")
		if err := recoverPartial(dir, id); err != nil {
			continue // a single unrecoverable partial must not block the rest
		}
		info, _ := os.Stat(finalPath(dir, id))
		sc := &Sidecar{
			FormatVersion: FormatVersion,
			EpisodeID:     id,
			Crashed:       true,
			Notes:         "recovered after unclean shutdown; re-footered from partial",
		}
		if info != nil {
			sc.Bytes = uint64(info.Size())
			sc.End = info.ModTime().UTC()
		}
		_ = sc.write(dir)
	}
	return nil
}

// recoverPartial re-lexes the footerless .partial and copies its schemas,
// channels, and messages into a fresh writer. Closing the writer emits the
// summary index, footer, and trailing magic so standard tooling can open it.
func recoverPartial(dir, id string) error {
	in, err := os.Open(partialPath(dir, id))
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := finalPath(dir, id) + ".recovering"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w, err := mcap.NewWriter(out, &mcap.WriterOptions{
		Chunked:     true,
		ChunkSize:   1 << 20,
		Compression: mcap.CompressionZSTD,
	})
	if err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}

	if err := w.WriteHeader(&mcap.Header{Profile: ""}); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := copyRecords(in, w); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := w.Close(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, finalPath(dir, id)); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Remove(partialPath(dir, id))
}

// copyRecords streams header, schemas, channels, and messages from a partial
// into the writer. The lexer de-chunks transparently, so a truncated trailing
// chunk simply stops the stream at the last intact record.
func copyRecords(in io.Reader, w *mcap.Writer) error {
	lexer, err := mcap.NewLexer(in)
	if err != nil {
		return err
	}
	defer lexer.Close()

	var buf []byte
	for {
		tok, rec, err := lexer.Next(buf)
		if err != nil {
			// EOF or a truncated trailing record ends the salvage cleanly; the
			// records read so far are valid and already copied.
			if errors.Is(err, io.EOF) {
				return nil
			}
			var trunc *mcap.ErrTruncatedRecord
			if errors.As(err, &trunc) {
				return nil
			}
			return err
		}
		if cap(rec) > cap(buf) {
			buf = rec[:0]
		}
		switch tok {
		case mcap.TokenSchema:
			s, err := mcap.ParseSchema(rec)
			if err != nil {
				return err
			}
			if err := w.WriteSchema(s); err != nil {
				return err
			}
		case mcap.TokenChannel:
			c, err := mcap.ParseChannel(rec)
			if err != nil {
				return err
			}
			if err := w.WriteChannel(c); err != nil {
				return err
			}
		case mcap.TokenMessage:
			m, err := mcap.ParseMessage(rec)
			if err != nil {
				return err
			}
			if err := w.WriteMessage(m); err != nil {
				return err
			}
		}
	}
}
