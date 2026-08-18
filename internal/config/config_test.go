package config

import "testing"

func TestDefaultValid(t *testing.T) {
	c := Default()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejects(t *testing.T) {
	c := Default()
	c.WheelSize = -1
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	c := Default()
	c.Workers = 8
	if err := c.Save(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Workers != 8 {
		t.Fatalf("expected 8, got %d", loaded.Workers)
	}
}
