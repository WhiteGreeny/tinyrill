package ippool

import "testing"

func TestExhaustionReturnsNil(t *testing.T) {
	p, err := NewIPPool[int]("198.18.0.0/30")
	if err != nil {
		t.Fatal(err)
	}
	if p.GenerateIP("a", 1) == nil || p.GenerateIP("b", 2) == nil || p.GenerateIP("c", 3) != nil {
		t.Fatal("unexpected allocation at exhaustion")
	}
	if p.GenerateIP("a", 1) == nil {
		t.Fatal("lost existing mapping")
	}
	for _, s := range []string{"::/64", "0.0.0.0/0", "10.0.0.0/32"} {
		if _, err := NewIPPool[int](s); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
