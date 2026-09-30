package workerbundle

import "testing"

func TestLinuxAMD64VerifiesEmbeddedChecksum(t *testing.T) {
	binary, checksum, err := LinuxAMD64()
	if err != nil {
		t.Fatal(err)
	}
	if len(binary) == 0 || len(checksum) != 64 {
		t.Fatalf("invalid bundled worker: size=%d checksum=%q", len(binary), checksum)
	}
}
