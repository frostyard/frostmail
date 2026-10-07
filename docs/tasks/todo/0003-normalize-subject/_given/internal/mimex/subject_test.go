package mimex

// CONTRACT TEST for task card T-0003 (docs/tasks). Do not edit.

import "testing"

func TestNormalizeSubject(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Lunch on Thursday?", "Lunch on Thursday?"},
		{"Re: Lunch on Thursday?", "Lunch on Thursday?"},
		{"RE: Lunch", "Lunch"},
		{"re:Lunch", "Lunch"},
		{"Re: Re: RE: Lunch", "Lunch"},
		{"Fwd: Lunch", "Lunch"},
		{"FW: Lunch", "Lunch"},
		{"Fw: Re: Lunch", "Lunch"},
		{"Re[2]: Lunch", "Lunch"},
		{"Re(3): Lunch", "Lunch"},
		{"Re : Lunch", "Lunch"},
		{"AW: Mittagessen", "Mittagessen"},
		{"WG: AW: Mittagessen", "Mittagessen"},
		{"SV: Lunsj", "Lunsj"},
		{"VS: Lounas", "Lounas"},
		{"Antw: Lunch", "Lunch"},
		{"RIF: Pranzo", "Pranzo"},
		{"TR: Déjeuner", "Déjeuner"},
		{"回复：午餐", "午餐"},
		{"答复: 转发：午餐", "午餐"},
		{"[dev] Re: Release plan", "Release plan"},
		{"Re: [dev] Re: Release plan", "Release plan"},
		{"[dev] [announce] Release plan", "Release plan"},
		{"Release [draft] plan", "Release [draft] plan"},
		{"  Re:\tLunch \r\n on   Thursday  ", "Lunch on Thursday"},
		{"Regarding: the lease", "Regarding: the lease"},
		{"Rebate offer", "Rebate offer"},
		{"Fwdx: not a prefix", "Fwdx: not a prefix"},
		{"Re:", ""},
		{"Re: Re: ", ""},
		{"", ""},
		{"[dev]", ""},
	}
	for _, tc := range cases {
		if got := NormalizeSubject(tc.in); got != tc.want {
			t.Errorf("NormalizeSubject(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeSubjectIsIdempotent(t *testing.T) {
	for _, s := range []string{"Re: [dev] Fwd: Plan", "AW: WG: Plan", "回复：计划", "Plan"} {
		once := NormalizeSubject(s)
		if twice := NormalizeSubject(once); twice != once {
			t.Errorf("NormalizeSubject(NormalizeSubject(%q)) = %q, want %q", s, twice, once)
		}
	}
}
