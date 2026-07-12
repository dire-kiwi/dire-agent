package threadstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"
)

func (d *ThreadDB) AppendMessage(ctx context.Context, message Message) (Message, error) {
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	result, err := d.database.ExecContext(ctx,
		`INSERT INTO messages(kind, role, content, data, created_at) VALUES(?, ?, ?, ?, ?)`,
		message.Kind, message.Role, message.Content, nullableJSON(message.Data), message.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Message{}, err
	}
	message.Sequence, _ = result.LastInsertId()
	return message, nil
}

// AppendMailboxMessage atomically appends a durable transcript message and
// marks it pending for one-time mailbox delivery.
func (d *ThreadDB) AppendMailboxMessage(ctx context.Context, message Message) (Message, error) {
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	tx, err := d.database.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx,
		`INSERT INTO messages(kind, role, content, data, created_at) VALUES(?, ?, ?, ?, ?)`,
		message.Kind, message.Role, message.Content, nullableJSON(message.Data), message.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Message{}, err
	}
	message.Sequence, err = result.LastInsertId()
	if err != nil {
		return Message{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_mailbox(message_sequence) VALUES(?)`, message.Sequence); err != nil {
		return Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return Message{}, err
	}
	return message, nil
}

// DrainMailboxMessages atomically claims and removes all pending mailbox
// markers while retaining their messages in the durable transcript.
func (d *ThreadDB) DrainMailboxMessages(ctx context.Context) ([]Message, error) {
	tx, err := d.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `DELETE FROM agent_mailbox RETURNING message_sequence`)
	if err != nil {
		return nil, err
	}
	var sequences []int64
	for rows.Next() {
		var sequence int64
		if err := rows.Scan(&sequence); err != nil {
			rows.Close()
			return nil, err
		}
		sequences = append(sequences, sequence)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })
	messages := make([]Message, 0, len(sequences))
	for _, sequence := range sequences {
		message, err := messageBySequence(ctx, tx, sequence)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return messages, nil
}

func messageBySequence(ctx context.Context, tx *sql.Tx, sequence int64) (Message, error) {
	var message Message
	var data []byte
	var created string
	err := tx.QueryRowContext(ctx,
		`SELECT sequence, kind, role, content, data, created_at FROM messages WHERE sequence = ?`, sequence).
		Scan(&message.Sequence, &message.Kind, &message.Role, &message.Content, &data, &created)
	if err != nil {
		return Message{}, err
	}
	message.Data = append(json.RawMessage(nil), data...)
	message.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return message, nil
}

func (d *ThreadDB) Messages(ctx context.Context, after int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := d.database.QueryContext(ctx,
		`SELECT sequence, kind, role, content, data, created_at FROM messages WHERE sequence > ? ORDER BY sequence LIMIT ?`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []Message
	for rows.Next() {
		var message Message
		var data []byte
		var created string
		if err := rows.Scan(&message.Sequence, &message.Kind, &message.Role, &message.Content, &data, &created); err != nil {
			return nil, err
		}
		message.Data = append(json.RawMessage(nil), data...)
		message.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (d *ThreadDB) AppendEvent(ctx context.Context, event Event) (Event, error) {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	result, err := d.database.ExecContext(ctx,
		`INSERT INTO events(type, data, created_at) VALUES(?, ?, ?)`,
		event.Type, nullableJSON(event.Data), event.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Event{}, err
	}
	event.Sequence, _ = result.LastInsertId()
	return event, nil
}

func (d *ThreadDB) Events(ctx context.Context, after int64, limit int) ([]Event, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := d.database.QueryContext(ctx,
		`SELECT sequence, type, data, created_at FROM events WHERE sequence > ? ORDER BY sequence LIMIT ?`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []Event
	for rows.Next() {
		var event Event
		var data []byte
		var created string
		if err := rows.Scan(&event.Sequence, &event.Type, &data, &created); err != nil {
			return nil, err
		}
		event.Data = append(json.RawMessage(nil), data...)
		event.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		events = append(events, event)
	}
	return events, rows.Err()
}

func nullableJSON(data json.RawMessage) any {
	if len(data) == 0 {
		return nil
	}
	return []byte(data)
}
