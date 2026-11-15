package help

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"telesrv/internal/domain"
)

type giftLimitUsers struct {
	verified bool
	err      error
}

func (u *giftLimitUsers) ByID(_ context.Context, id int64) (domain.User, bool, error) {
	return domain.User{ID: id, Verified: u.verified}, true, u.err
}

func TestGiftPinLimitTracksVerificationAndInvalidatesConfigHash(t *testing.T) {
	users := &giftLimitUsers{}
	svc := NewService(nil, nil, WithAccountUsers(users))
	check := func(hash, limit int, wantSame bool) domain.AppConfig {
		t.Helper()
		cfg, same, err := svc.GetAppConfig(context.Background(), 1001, hash)
		if err != nil || same != wantSame {
			t.Fatalf("same=%v err=%v", same, err)
		}
		var values map[string]json.RawMessage
		if err := json.Unmarshal(cfg.JSON, &values); err != nil {
			t.Fatal(err)
		}
		var got int
		if err := json.Unmarshal(values["stargifts_pinned_to_top_limit"], &got); err != nil || got != limit {
			t.Fatalf("limit=%d want=%d err=%v", got, limit, err)
		}
		return cfg
	}
	normal := check(0, domain.MaxPinnedStarGifts, false)
	users.verified = true
	verified := check(normal.Hash, domain.MaxPinnedStarGiftsVerified, false)
	check(verified.Hash, domain.MaxPinnedStarGiftsVerified, true)
	users.verified = false
	check(verified.Hash, domain.MaxPinnedStarGifts, false)
	users.err = errors.New("unavailable")
	if _, _, err := svc.GetAppConfig(context.Background(), 1001, verified.Hash); err == nil {
		t.Fatal("ignored user lookup error")
	}
}
