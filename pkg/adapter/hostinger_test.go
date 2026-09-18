package adapter

import (
	"os"
	"testing"

	"github.com/makemore/machine/pkg/machinefile"
)

func TestHostingerMapItemID_Defaults8GBToKVM2(t *testing.T) {
	t.Setenv("HOSTINGER_VPS_ITEM_ID", "")
	t.Setenv("HOSTINGER_CURRENCY", "")
	t.Setenv("HOSTINGER_BILLING_PERIOD", "")
	mf := &machinefile.Machinefile{Resources: machinefile.Resources{CPU: 2, Memory: "8gb"}}
	got := NewHostingerAdapter().mapItemID(mf)
	want := "hostingercom-vps-kvm2-usd-1m"
	if got != want {
		t.Fatalf("mapItemID() = %q, want %q", got, want)
	}
}

func TestHostingerMapItemID_EnvOverride(t *testing.T) {
	t.Setenv("HOSTINGER_VPS_ITEM_ID", "custom-plan")
	mf := &machinefile.Machinefile{Resources: machinefile.Resources{Memory: "8gb"}}
	if got := NewHostingerAdapter().mapItemID(mf); got != "custom-plan" {
		t.Fatalf("mapItemID() = %q, want env override", got)
	}
}

func TestHostingerEnvOrNumeric(t *testing.T) {
	os.Unsetenv("HOSTINGER_TEMPLATE_ID")
	if id, ok, err := envOrNumeric("HOSTINGER_TEMPLATE_ID", "1130"); err != nil || !ok || id != 1130 {
		t.Fatalf("envOrNumeric numeric raw = (%d,%v,%v), want (1130,true,nil)", id, ok, err)
	}
}

func TestHostingerFirstMatchingID(t *testing.T) {
	items := []hostingerNamedID{{ID: 19, Name: "fra", City: "Frankfurt", Continent: "Europe"}}
	id, err := firstMatchingID(items, "europe", "data center", "HOSTINGER_DATA_CENTER_ID")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 19 {
		t.Fatalf("firstMatchingID() = %d, want 19", id)
	}
}
