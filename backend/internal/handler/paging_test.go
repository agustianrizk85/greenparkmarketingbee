package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"marketingflow/internal/model"
)

func TestMintaPagingTanpaPage(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/work-items?q=x", nil)
	if _, ok := mintaPaging(r); ok {
		t.Fatal("tanpa page harus ok=false (balasan lama)")
	}
	r = httptest.NewRequest("GET", "/api/work-items?page=0&limit=9999&q=%20Reels%20&sort=title&dir=DESC", nil)
	p, ok := mintaPaging(r)
	if !ok || p.Page != 1 || p.Limit != pagingLimitMaks || p.Q != "reels" || p.Sort != "title" || !p.Desc {
		t.Fatalf("paging salah dibaca: %+v ok=%v", p, ok)
	}
}

func TestHalamanWorkItem(t *testing.T) {
	now := time.Now()
	items := []model.WorkItem{
		{ID: 1, Title: "Reels Limo", Project: "ZHL", CreatedAt: now},
		{ID: 2, Title: "Feed Verua", Project: "Verua", CreatedAt: now.Add(time.Hour)},
		{ID: 10, Title: "Reels Verua", Project: "Verua", CreatedAt: now.Add(2 * time.Hour)},
	}
	got := halaman(items, PageQuery{Page: 1, Limit: 1, Q: "reels", Sort: "id", Desc: true}, kolomWorkItem)
	if got.Total != 2 || len(got.Items) != 1 || got.Items[0].ID != 10 {
		t.Fatalf("q+sort id desc salah: %+v", got)
	}
	got = halaman(items, PageQuery{Page: 2, Limit: 2, Sort: "created_at"}, kolomWorkItem)
	if got.Total != 3 || len(got.Items) != 1 || got.Items[0].ID != 10 {
		t.Fatalf("halaman 2 salah: %+v", got)
	}
	got = halaman(items, PageQuery{Page: 9, Limit: 25}, kolomWorkItem)
	if got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("halaman di luar jangkauan harus [] bukan null: %+v", got)
	}
}
