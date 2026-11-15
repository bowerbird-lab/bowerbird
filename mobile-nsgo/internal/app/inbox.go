package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/owocc/bowerbird/mobile-nsgo/internal/model"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/platform"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/storage"
	"github.com/owocc/bowerbird/mobile-nsgo/internal/transport"
)

// Inbox owns the phone's pending shares. It persists a host event before
// acknowledging it, so a killed process can replay an event without creating a
// second entry and cannot lose content the user shared.
type Inbox struct {
	local *storage.Local
}

// NewInbox wraps the app's local documents.
func NewInbox(local *storage.Local) *Inbox { return &Inbox{local: local} }

// Entries returns the inbox newest first.
func (i *Inbox) Entries() ([]storage.InboxEntry, error) {
	idx, err := i.local.LoadInbox()
	if err != nil {
		return nil, err
	}
	out := append([]storage.InboxEntry(nil), idx.Entries...)
	return out, nil
}

// Count returns the number of pending entries.
func (i *Inbox) Count() int {
	entries, err := i.Entries()
	if err != nil {
		return 0
	}
	return len(entries)
}

// Consume takes pending host events, copies their content into app storage,
// appends index entries and only then acknowledges the events.
func (i *Inbox) Consume(ctx context.Context, intake platform.ShareIntake, done func(added int, err error)) {
	if intake == nil {
		done(0, platform.ErrUnavailable)
		return
	}
	intake.TakePending(func(events []platform.ShareEvent, err error) {
		if err != nil {
			done(0, err)
			return
		}
		added, acked, consumeErr := i.consumeEvents(ctx, events)
		if len(acked) > 0 {
			intake.Ack(acked, func(ackErr error) {
				if consumeErr == nil {
					consumeErr = ackErr
				}
				done(added, consumeErr)
			})
			return
		}
		done(added, consumeErr)
	})
}

func (i *Inbox) consumeEvents(ctx context.Context, events []platform.ShareEvent) (int, []string, error) {
	if len(events) == 0 {
		return 0, nil, nil
	}
	idx, err := i.local.LoadInbox()
	if err != nil {
		return 0, nil, err
	}
	processed := make(map[string]bool, len(idx.ProcessedEvents))
	for _, id := range idx.ProcessedEvents {
		processed[id] = true
	}
	known := make(map[string]bool, len(idx.Entries))
	for _, entry := range idx.Entries {
		if entry.EventID != "" {
			known[entry.EventID] = true
		}
	}

	added := 0
	var acked []string
	for _, event := range events {
		if ctx.Err() != nil {
			return added, acked, ctx.Err()
		}
		id := strings.TrimSpace(event.ID)
		if id == "" {
			continue
		}
		if processed[id] || known[id] {
			acked = append(acked, id)
			continue
		}
		entries, copyErr := i.persistEvent(id, event.Items)
		if copyErr != nil {
			continue
		}
		idx.Entries = append(entries, idx.Entries...)
		idx.ProcessedEvents = append(idx.ProcessedEvents, id)
		if err := i.local.SaveInbox(idx); err != nil {
			return added, acked, err
		}
		processed[id] = true
		known[id] = true
		added += len(entries)
		acked = append(acked, id)
	}
	return added, acked, nil
}

func (i *Inbox) persistEvent(eventID string, items []platform.SharedItem) ([]storage.InboxEntry, error) {
	out := make([]storage.InboxEntry, 0, len(items))
	now := time.Now().UnixMilli()
	for n, item := range items {
		entry := storage.InboxEntry{
			ID:        fmt.Sprintf("%s-%d", eventID, n),
			CreatedAt: now,
			Source:    "share",
			EventID:   eventID,
			Name:      strings.TrimSpace(item.Name),
			MimeType:  item.MimeType,
			Text:      item.Text,
		}
		switch strings.ToLower(item.Kind) {
		case "text":
			entry.Kind = "text"
			if entry.Name == "" {
				entry.Name = "分享文本.txt"
			}
			entry.Size = int64(len(item.Text))
		default:
			entry.Kind = "file"
			path := strings.TrimSpace(item.Path)
			if path == "" {
				continue
			}
			src, err := os.Open(path)
			if err != nil {
				return out, err
			}
			rel := filepath.Join("share_inbox", "media", safeDir(eventID), safeName(entry.Name, n))
			size, writeErr := i.local.Store().WriteReader(rel, src, 0o600)
			_ = src.Close()
			if writeErr != nil {
				return out, writeErr
			}
			entry.StoredPath = filepath.ToSlash(rel)
			entry.Size = size
			if entry.Name == "" {
				entry.Name = filepath.Base(rel)
			}
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return nil, errors.New("shared event has no usable content")
	}
	return out, nil
}

// Upload sends one entry and returns the upload result along with the
// uppercase SHA-256 hex of the uploaded bytes for phone-share tagging.
func (i *Inbox) Upload(ctx context.Context, client *transport.Client, entry storage.InboxEntry) (model.UploadResult, string, error) {
	var result model.UploadResult
	var shaHex string
	var err error
	if entry.Kind == "text" {
		name := entry.Name
		if name == "" {
			name = "分享文本.txt"
		}
		if !strings.HasSuffix(strings.ToLower(name), ".txt") {
			name += ".txt"
		}
		sum := sha256.Sum256([]byte(entry.Text))
		shaHex = strings.ToUpper(hex.EncodeToString(sum[:]))
		result, err = client.Upload(ctx, name, strings.NewReader(entry.Text), int64(len(entry.Text)))
	} else {
		path, pathErr := i.local.Store().Path(entry.StoredPath)
		if pathErr != nil {
			return result, "", pathErr
		}
		shaHex, _ = hashFileUpper(path)
		result, err = client.UploadFile(ctx, path)
	}
	if err != nil {
		return result, "", err
	}
	if err := i.Remove(entry); err != nil {
		return result, shaHex, err
	}
	return result, shaHex, nil
}

func hashFileUpper(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil))), nil
}

// Remove deletes one entry and its private file.
func (i *Inbox) Remove(entry storage.InboxEntry) error {
	idx, err := i.local.LoadInbox()
	if err != nil {
		return err
	}
	out := idx.Entries[:0]
	for _, old := range idx.Entries {
		if old.ID != entry.ID {
			out = append(out, old)
		}
	}
	idx.Entries = out
	if err := i.local.SaveInbox(idx); err != nil {
		return err
	}
	if entry.StoredPath != "" {
		if path, pathErr := i.local.Store().Path(entry.StoredPath); pathErr == nil {
			_ = os.Remove(path)
		}
	}
	return nil
}

func safeDir(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == 0 {
			return '_'
		}
		return r
	}, s)
	if s == "" {
		return "event"
	}
	return s
}

func safeName(name string, fallback int) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		name = fmt.Sprintf("shared-%d", fallback+1)
	}
	return name
}
