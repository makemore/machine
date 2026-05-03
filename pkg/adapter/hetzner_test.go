package adapter

import "testing"

func TestSSHKeyBody(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "with trailing comment",
			in:   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleBody user@host",
			want: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleBody",
		},
		{
			name: "no comment",
			in:   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleBody",
			want: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleBody",
		},
		{
			name: "leading and trailing whitespace",
			in:   "  ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleBody root@builder  \n",
			want: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleBody",
		},
		{
			name: "rsa with multi-word comment",
			in:   "ssh-rsa AAAAB3NzaC1yc2EAAA chris on laptop",
			want: "ssh-rsa AAAAB3NzaC1yc2EAAA",
		},
		{
			name: "single field falls back to whole string",
			in:   "garbage",
			want: "garbage",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sshKeyBody(tc.in)
			if got != tc.want {
				t.Errorf("sshKeyBody(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSSHKeyBody_EquivalenceAcrossComments(t *testing.T) {
	// The whole point: two functionally identical keys with different
	// comments must produce the same body.
	a := "ssh-ed25519 AAAAC3SAMEKEY user@laptop"
	b := "ssh-ed25519 AAAAC3SAMEKEY ci-runner@cloudbuild"
	if sshKeyBody(a) != sshKeyBody(b) {
		t.Fatalf("equivalent keys produced different bodies:\n  a=%q\n  b=%q", sshKeyBody(a), sshKeyBody(b))
	}
}
