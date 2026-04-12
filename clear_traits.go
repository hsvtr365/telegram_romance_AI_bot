//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	dsn := "postgres://heartlink:rlatnwlWkdWkdaos1!@158.180.81.49:5432/heartlink_db?sslmode=disable"
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Println("Error connecting to db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	tag, err := pool.Exec(ctx, "DELETE FROM tg_user_traits")
	if err != nil {
		fmt.Println("Error deleting traits:", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully deleted %d old user traits.\n", tag.RowsAffected())
}
