// Package entity provides the foundational building block for all entities
// in the wolf-be domain model. Entities are distinguished by identity rather
// than by their attribute values.
package entity

import "time"

// Base holds the common identity and audit fields shared by all entities.
// Embed Base in domain entity structs to inherit ID and timestamp tracking.
type Base struct {
	id        string
	createdAt time.Time
	updatedAt time.Time
}

// NewBase creates a new Base with the given string identifier and
// both timestamps set to the current UTC time.
func NewBase(id string) Base {
	now := time.Now().UTC()
	return Base{
		id:        id,
		createdAt: now,
		updatedAt: now,
	}
}

// ID returns the entity's unique string identifier.
func (b *Base) ID() string { return b.id }

// CreatedAt returns the UTC time at which the entity was created.
func (b *Base) CreatedAt() time.Time { return b.createdAt }

// UpdatedAt returns the UTC time at which the entity was last modified.
func (b *Base) UpdatedAt() time.Time { return b.updatedAt }

// SetUpdatedAt sets the entity's last-modified timestamp.
func (b *Base) SetUpdatedAt(t time.Time) { b.updatedAt = t }
