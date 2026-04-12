//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/hsvtr365/telegram_romance_AI_bot/internal/store/postgres"
)

func main() {
	ctx := context.Background()
	dsn := "postgres://heartlink:rlatnwlWkdWkdaos1!@158.180.81.49:5432/heartlink_db?sslmode=disable"
	store, err := postgres.New(ctx, dsn)
	if err != nil {
		fmt.Println("failed:", err)
		os.Exit(1)
	}
	defer store.Close()

    schema := `DELETE FROM tg_user_traits;`
    // Wait, the store object has a pool, but it's private. Let's just use pgxpool directly.
}
