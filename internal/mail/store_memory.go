package mail

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

type MemoryStore struct {
	mu          sync.RWMutex
	mailboxes   map[string]Mailbox
	threads     map[string]Thread
	messages    map[string]Message
	attachments map[string][]Attachment
	events      map[string]DeliveryEvent
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		mailboxes:   map[string]Mailbox{},
		threads:     map[string]Thread{},
		messages:    map[string]Message{},
		attachments: map[string][]Attachment{},
		events:      map[string]DeliveryEvent{},
	}
}

func (s *MemoryStore) UpsertMailbox(_ context.Context, mailbox Mailbox) (Mailbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for id, existing := range s.mailboxes {
		if existing.EmailAddress == mailbox.EmailAddress {
			mailbox.ID = id
			if mailbox.CreatedAt.IsZero() {
				mailbox.CreatedAt = existing.CreatedAt
			}
			mailbox.UpdatedAt = now
			s.mailboxes[id] = mailbox
			return mailbox, nil
		}
	}
	if mailbox.ID == "" {
		mailbox.ID = uuid.NewString()
	}
	if mailbox.CreatedAt.IsZero() {
		mailbox.CreatedAt = now
	}
	mailbox.UpdatedAt = now
	s.mailboxes[mailbox.ID] = mailbox
	return mailbox, nil
}

func (s *MemoryStore) ListMailboxes(_ context.Context, accountID string) ([]Mailbox, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Mailbox
	for _, mailbox := range s.mailboxes {
		if mailbox.AccountID == accountID {
			out = append(out, mailbox)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EmailAddress < out[j].EmailAddress })
	return out, nil
}

func (s *MemoryStore) GetMailbox(_ context.Context, accountID, mailboxID string) (Mailbox, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	mailbox, ok := s.mailboxes[mailboxID]
	if !ok || mailbox.AccountID != accountID {
		return Mailbox{}, errors.New("mailbox not found")
	}
	return mailbox, nil
}

func (s *MemoryStore) FindMailboxByAddress(_ context.Context, emailAddress string) (Mailbox, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, mailbox := range s.mailboxes {
		if mailbox.EmailAddress == emailAddress {
			return mailbox, nil
		}
	}
	return Mailbox{}, errors.New("mailbox not found")
}

func (s *MemoryStore) UpsertThread(_ context.Context, thread Thread) (Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for id, existing := range s.threads {
		if existing.AccountID == thread.AccountID && existing.MailboxID == thread.MailboxID && existing.ThreadKey == thread.ThreadKey {
			thread.ID = id
			if thread.CreatedAt.IsZero() {
				thread.CreatedAt = existing.CreatedAt
			}
			thread.UpdatedAt = now
			s.threads[id] = thread
			return thread, nil
		}
	}
	if thread.ID == "" {
		thread.ID = uuid.NewString()
	}
	if thread.CreatedAt.IsZero() {
		thread.CreatedAt = now
	}
	thread.UpdatedAt = now
	s.threads[thread.ID] = thread
	return thread, nil
}

func (s *MemoryStore) CreateMessage(_ context.Context, message Message, attachments []Attachment) (Message, []Attachment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if message.ID == "" {
		message.ID = uuid.NewString()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	message.UpdatedAt = now
	s.messages[message.ID] = message
	outAttachments := make([]Attachment, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.ID == "" {
			attachment.ID = uuid.NewString()
		}
		attachment.MessageID = message.ID
		if attachment.CreatedAt.IsZero() {
			attachment.CreatedAt = now
		}
		outAttachments = append(outAttachments, attachment)
	}
	s.attachments[message.ID] = outAttachments
	return message, outAttachments, nil
}

func (s *MemoryStore) ListMessages(_ context.Context, accountID, mailboxID string, limit int) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Message
	for _, message := range s.messages {
		if message.AccountID == accountID && message.MailboxID == mailboxID {
			out = append(out, message)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryStore) GetMessage(_ context.Context, accountID, messageID string) (Message, []Attachment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	message, ok := s.messages[messageID]
	if !ok || message.AccountID != accountID {
		return Message{}, nil, errors.New("message not found")
	}
	return message, append([]Attachment(nil), s.attachments[messageID]...), nil
}

func (s *MemoryStore) RecordDeliveryEvent(_ context.Context, event DeliveryEvent) (DeliveryEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	s.events[event.ID] = event
	return event, nil
}
