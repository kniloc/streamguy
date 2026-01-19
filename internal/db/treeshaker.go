package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func UpdateNumberOfTurns(ctx context.Context, pool *pgxpool.Pool, turns int, username string) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO user_game (name, turns_left) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET turns_left = user_game.turns_left + $2",
		username, turns)
	return err
}
