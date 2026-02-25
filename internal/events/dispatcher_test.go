package events

import (
	"testing"
)

func TestMatchesEvent(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		event    string
		want     bool
	}{
		{"empty patterns match all", nil, "lease.ack", true},
		{"exact match", []string{"lease.ack"}, "lease.ack", true},
		{"exact no match", []string{"lease.ack"}, "lease.release", false},
		{"wildcard all", []string{"*"}, "anything", true},
		{"wildcard prefix", []string{"lease.*"}, "lease.ack", true},
		{"wildcard prefix match release", []string{"lease.*"}, "lease.release", true},
		{"wildcard prefix no match", []string{"lease.*"}, "conflict.detected", false},
		{"multiple patterns", []string{"lease.ack", "conflict.*"}, "conflict.detected", true},
		{"multiple patterns no match", []string{"lease.ack", "ha.*"}, "conflict.detected", false},
		{"conflict wildcard", []string{"conflict.*"}, "conflict.permanent", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesEvent(tt.patterns, tt.event)
			if got != tt.want {
				t.Errorf("matchesEvent(%v, %q) = %v, want %v", tt.patterns, tt.event, got, tt.want)
			}
		})
	}
}

func TestMatchesSubnet(t *testing.T) {
	tests := []struct {
		name    string
		subnets []string
		evt     Event
		want    bool
	}{
		{"empty subnets match all", nil, Event{}, true},
		{"no subnet in event matches all", []string{"192.168.1.0/24"}, Event{}, true},
		{"matching subnet", []string{"192.168.1.0/24"}, Event{Lease: &LeaseData{Subnet: "192.168.1.0/24"}}, true},
		{"non-matching subnet", []string{"10.0.0.0/24"}, Event{Lease: &LeaseData{Subnet: "192.168.1.0/24"}}, false},
		{"conflict subnet match", []string{"192.168.1.0/24"}, Event{Conflict: &ConflictData{Subnet: "192.168.1.0/24"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesSubnet(tt.subnets, tt.evt)
			if got != tt.want {
				t.Errorf("matchesSubnet(%v, ...) = %v, want %v", tt.subnets, got, tt.want)
			}
		})
	}
}
