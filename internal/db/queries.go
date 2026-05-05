package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func UpdateNumberOfTurns(ctx context.Context, pool *pgxpool.Pool, turns int, userID string, username string) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO user_game (user_id, name, turns_left) VALUES ($1, $2, $3) ON CONFLICT (user_id) DO UPDATE SET turns_left = user_game.turns_left + $3, name = $2",
		userID, username, turns)
	return err
}

func AddObtainedPlate(ctx context.Context, pool *pgxpool.Pool, userId, userName, plateRegion, plateNumber, textColor string) error {
	_, pError := pool.Exec(ctx,
		"INSERT INTO license_plates (user_id, username, plate_region, plate_number, text_color) values ($1, $2, $3, $4, $5)", userId, userName, plateRegion, plateNumber, textColor)
	return pError
}
