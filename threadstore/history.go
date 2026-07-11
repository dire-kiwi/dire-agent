package threadstore

import (
	"context"
	"encoding/json"
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
