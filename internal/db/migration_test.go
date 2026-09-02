//go:build integration

package store

import (
	"strings"
	"testing"
)

func TestAccountBalanceConstraintAndTransferIndex(t *testing.T) {
	owner := createTestUser(t)
	account := createTestAccount(t, owner.Username)

	if _, err := testPool.Exec(t.Context(), "UPDATE accounts SET balance = -1 WHERE id = $1", account.ID); err == nil {
		t.Fatal("negative account balance update succeeded")
	}

	var indexDefinition string
	if err := testPool.QueryRow(t.Context(), `
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = current_schema() AND indexname = 'idx_transfers_from_account_created_at'
	`).Scan(&indexDefinition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(indexDefinition, "(from_account_id, created_at)") {
		t.Fatalf("unexpected transfer index definition: %s", indexDefinition)
	}
}
