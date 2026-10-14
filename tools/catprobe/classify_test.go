package main

import "testing"

func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		name string
		mail Mail
		want Category
	}{
		{"a person", Mail{From: "ann@x.test", Subject: "Lunch?"}, Primary},
		{"a known sender's receipt", Mail{From: "ann@x.test", Subject: "Your receipt", Unsubscribe: true, Known: true}, Primary},
		{"a shop's receipt", Mail{From: "orders@shop.test", Subject: "Your order has shipped", Unsubscribe: true}, Transactions},
		{"a machine's receipt", Mail{From: "no-reply@bank.test", Subject: "Your statement is ready"}, Transactions},
		{"a person's order", Mail{From: "bob@x.test", Subject: "In order to meet"}, Primary},
		{"a mailing list", Mail{From: "list@lists.test", Subject: "[dev] release", ListID: true, Unsubscribe: true}, Updates},
		{"a newsletter", Mail{From: "hello@brand.test", Subject: "40% off this week", Unsubscribe: true}, Promotions},
		{"a notice", Mail{From: "notifications@service.test", Subject: "New sign-in"}, Updates},
		{"a tagged notice", Mail{From: "noreply+alerts@service.test", Subject: "Alert"}, Updates},
		{"not a machine", Mail{From: "information@x.test", Subject: "Hello"}, Primary},
	} {
		if got := Classify(tc.mail); got != tc.want {
			t.Errorf("%s: Classify = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestFromGmail(t *testing.T) {
	for _, tc := range []struct {
		cats map[string]bool
		want Category
		ok   bool
	}{
		{map[string]bool{"primary": true}, Primary, true},
		{map[string]bool{"social": true}, Updates, true},
		{map[string]bool{"forums": true}, Updates, true},
		{map[string]bool{"promotions": true}, Promotions, true},
		{map[string]bool{"updates": true, "purchases": true}, Transactions, true},
		{map[string]bool{}, Primary, false},
	} {
		if got, ok := fromGmail(tc.cats); got != tc.want || ok != tc.ok {
			t.Errorf("fromGmail(%v) = %v, %v; want %v, %v", tc.cats, got, ok, tc.want, tc.ok)
		}
	}
}
