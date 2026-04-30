package mail

import "context"

type Store interface {
	UpsertMailbox(ctx context.Context, mailbox Mailbox) (Mailbox, error)
	ListMailboxes(ctx context.Context, accountID string) ([]Mailbox, error)
	GetMailbox(ctx context.Context, accountID, mailboxID string) (Mailbox, error)
	FindMailboxByAddress(ctx context.Context, emailAddress string) (Mailbox, error)

	UpsertThread(ctx context.Context, thread Thread) (Thread, error)
	CreateMessage(ctx context.Context, message Message, attachments []Attachment) (Message, []Attachment, error)
	ListMessages(ctx context.Context, accountID, mailboxID string, limit int) ([]Message, error)
	GetMessage(ctx context.Context, accountID, messageID string) (Message, []Attachment, error)
	RecordDeliveryEvent(ctx context.Context, event DeliveryEvent) (DeliveryEvent, error)
}
