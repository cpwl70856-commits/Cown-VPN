package tunnel

import "testing"

func TestExpandAllowedForModeSplit(t *testing.T) {
	dests, full := expandAllowedForMode([]string{"10.8.0.0/24", "0.0.0.0/0"}, RouteModeSplit)
	if full {
		t.Fatal("split mode should not mark full when not applying global")
	}
	if len(dests) != 1 || dests[0] != "10.8.0.0/24" {
		t.Fatalf("dests=%v", dests)
	}
}

func TestExpandAllowedForModeGlobal(t *testing.T) {
	dests, full := expandAllowedForMode([]string{"10.8.0.0/24"}, RouteModeGlobal)
	if !full {
		t.Fatal("expected full")
	}
	if len(dests) != 2 {
		t.Fatalf("want half-routes, got %v", dests)
	}
}

func TestExpandAllowedForModeNone(t *testing.T) {
	dests, full := expandAllowedForMode([]string{"0.0.0.0/0", "10.8.0.0/24"}, RouteModeNone)
	if full || len(dests) != 0 {
		t.Fatalf("none mode must install no routes, got full=%v dests=%v", full, dests)
	}
}

func TestHasFullTunnel(t *testing.T) {
	if !hasFullTunnel([]string{"0.0.0.0/0"}) {
		t.Fatal("expected full")
	}
	if hasFullTunnel([]string{"10.0.0.0/8"}) {
		t.Fatal("not full")
	}
}

func TestDescribeRoutes(t *testing.T) {
	if DescribeRoutes(nil) == "" {
		t.Fatal("empty describe")
	}
	s := DescribeRoutes([]AppliedRoute{{Destination: "10.8.0.0/24", Iface: "wg0", Kind: "net"}})
	if s == "" {
		t.Fatal("expected summary")
	}
}
