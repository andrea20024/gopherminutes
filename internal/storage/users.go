package storage

import (
	"context"
	"fmt"
	"time"
)

// User represents a user in the system.
type User struct {
	ID        int       `json:"id"`
	Username  *string   `json:"username,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// UserRepo handles user data operations.
type UserRepo struct {
	*Repository
}

// NewUserRepo creates a new UserRepo.
func NewUserRepo(r *Repository) *UserRepo {
	return &UserRepo{Repository: r}
}

// CreateUser creates a new user with the given ID and returns it.
func (r *UserRepo) CreateUser(ctx context.Context, id int, username string) (*User, error) {
	var user User
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO users (id, username) VALUES ($1, $2)
		RETURNING id, username, created_at
	`, id, username).Scan(&user.ID, &user.Username, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &user, nil
}

// GetUserByID retrieves a user by their internal ID.
func (r *UserRepo) GetUserByID(ctx context.Context, id int) (*User, error) {
	var user User
	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, created_at FROM users WHERE id = $1
	`, id).Scan(&user.ID, &user.Username, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &user, nil
}
