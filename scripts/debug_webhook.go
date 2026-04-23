package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := "4PBMBKere4V2gKE.root:HuwQ9GVbm9kIwjeR@tcp(gateway01.ap-southeast-1.prod.aws.tidbcloud.com:4000)/?tls=true&parseTime=true"

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	fmt.Println("Listing all databases:")
	rows, err := db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		log.Fatalf("failed to query databases: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		rows.Scan(&name)
		fmt.Printf(" - %s\n", name)
	}
}
