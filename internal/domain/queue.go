package domain

import "context"

type EventQueue interface {
	Push(ctx context.Context, event *Event) error
}