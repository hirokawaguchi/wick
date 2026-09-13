package config

import (
	"testing"
	"time"
)

func TestApplyTimeZoneTokyo(t *testing.T) {
	c := Config{TimeZone: "Asia/Tokyo"}
	loc, _ := c.ApplyTimeZone()
	if loc == nil {
		t.Fatal("nil loc")
	}
	_, off := time.Now().In(loc).Zone()
	if off != 9*60*60 {
		t.Fatalf("offset %d", off)
	}
	if time.Local != loc {
		t.Fatal("time.Local not set")
	}
}
