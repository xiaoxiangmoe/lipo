package lipo_test

import (
	"path/filepath"
	"testing"

	"github.com/konoui/lipo/pkg/lipo"
)

func TestLipo_Remove(t *testing.T) {
	tests := []struct {
		name   string
		inputs []string
		arches []string
	}{
		{
			name:   "-remove x86_64",
			inputs: []string{inAmd64, inArm64, "arm64e"},
			arches: []string{"x86_64"},
		},
		{
			name:   "-remove arm64",
			inputs: []string{inAmd64, inArm64, "arm64e"},
			arches: []string{"arm64"},
		},
		{
			name:   "-remove arm64 -remove arm64e",
			inputs: []string{inAmd64, inArm64, "arm64e"},
			arches: []string{"arm64", "arm64e"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := setup(t, tt.inputs...)

			got := filepath.Join(p.dir, randName())
			arches := tt.arches
			l := lipo.New(lipo.WithInputs(p.fatBin), lipo.WithOutput(got))
			if err := l.Remove(arches...); err != nil {
				t.Errorf("remove error %v\n", err)
			}

			wantArches := []string{}
			for _, i := range tt.inputs {
				if !contain(i, tt.arches) {
					wantArches = append(wantArches, i)
				}
			}
			verifyArches(t, got, wantArches...)

			if p.skip() {
				t.Skip("skip lipo binary tests")
			}

			want := filepath.Join(p.dir, randName())
			p.remove(t, want, p.fatBin, arches)
			diffSha256(t, want, got)
		})
	}
}
