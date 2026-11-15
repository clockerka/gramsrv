package postgres

import (
	"context"
	"testing"

	"telesrv/deploy"
)

func TestReconcileImportedProfileFieldsPostgres(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	read := func(name string) string {
		t.Helper()
		raw, err := deploy.Migrations.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	// Simulate an installation which had already passed the timestamps at which
	// the imported migrations were added. Transaction rollback restores the schema.
	for _, name := range []string{
		"20261104010000_user_custom_label.down.sql",
		"20261105010000_channel_collectible_emoji_status.down.sql",
	} {
		if _, err := tx.Exec(ctx, read(name)); err != nil {
			t.Fatal(err)
		}
	}
	repair := read("20261115010000_reconcile_imported_profile_fields.up.sql")
	for pass := 0; pass < 2; pass++ {
		if _, err := tx.Exec(ctx, repair); err != nil {
			t.Fatalf("repair pass %d: %v", pass, err)
		}
	}
	if _, err := tx.Exec(ctx, read("20261115010000_reconcile_imported_profile_fields.down.sql")); err != nil {
		t.Fatal(err)
	}
	var columns int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
 WHERE table_schema='public' AND (table_name, column_name) IN (
 ('users', 'custom_label'), ('channels', 'emoji_status_collectible_id'),
 ('channels', 'emoji_status_collectible'))`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 3 {
		t.Fatalf("restored columns=%d, want 3 after repair and its rollback", columns)
	}
	var reference bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint
 WHERE conrelid='public.channels'::regclass AND confrelid='public.unique_star_gifts'::regclass
 AND contype='f' AND conkey=ARRAY[(SELECT attnum FROM pg_attribute
 WHERE attrelid='public.channels'::regclass AND attname='emoji_status_collectible_id')])`).Scan(&reference); err != nil {
		t.Fatal(err)
	}
	if !reference {
		t.Fatal("collectible status foreign key was not restored")
	}
}
