package freshdesk

import (
	"strings"
	"time"
)

// filterTickets applies a TicketFilter to a slice of tickets and returns matches.
func filterTickets(tickets []Ticket, filter *TicketFilter) []Ticket {
	now := time.Now().UTC()
	var matches []Ticket
	for i := range tickets {
		if ticketMatchesFilter(&tickets[i], filter, now) {
			matches = append(matches, tickets[i])
		}
	}
	return matches
}

// ticketMatchesFilter reports whether a single ticket satisfies every set criterion in the filter.
//
//nolint:cyclop // flat sequence of independent field guards; cyclomatic count overstates complexity
func ticketMatchesFilter(t *Ticket, filter *TicketFilter, now time.Time) bool {
	if filter.Query != "" {
		q := strings.ToLower(filter.Query)
		if !strings.Contains(strings.ToLower(t.Subject), q) &&
			!strings.Contains(strings.ToLower(t.Type), q) &&
			!strings.Contains(strings.ToLower(t.DescriptionText), q) {
			return false
		}
	}
	if filter.Status != 0 && t.Status != filter.Status {
		return false
	}
	if filter.Priority != 0 && t.Priority != filter.Priority {
		return false
	}
	if filter.Type != "" && !strings.EqualFold(t.Type, filter.Type) {
		return false
	}
	if filter.IsEscalated && !t.IsEscalated {
		return false
	}
	if filter.RequesterID != 0 && t.RequesterID != filter.RequesterID {
		return false
	}
	if filter.CompanyID != 0 && t.CompanyID != filter.CompanyID {
		return false
	}
	if filter.GroupID != 0 && t.GroupID != filter.GroupID {
		return false
	}
	if filter.AgentID != 0 && t.ResponderID != filter.AgentID {
		return false
	}
	if !matchesCreatedRange(t, filter) {
		return false
	}
	if filter.Overdue && !isOverdue(t, now) {
		return false
	}
	return true
}

// matchesCreatedRange reports whether the ticket's creation time falls within the filter's
// CreatedAfter/CreatedBefore bounds. Unparseable bounds or timestamps are treated as no constraint.
func matchesCreatedRange(t *Ticket, filter *TicketFilter) bool {
	if filter.CreatedAfter != "" {
		after, err := time.Parse(time.RFC3339, filter.CreatedAfter)
		created, cerr := time.Parse(time.RFC3339, t.CreatedAt)
		if err == nil && cerr == nil && created.Before(after) {
			return false
		}
	}
	if filter.CreatedBefore != "" {
		before, err := time.Parse(time.RFC3339, filter.CreatedBefore)
		created, cerr := time.Parse(time.RFC3339, t.CreatedAt)
		if err == nil && cerr == nil && created.After(before) {
			return false
		}
	}
	return true
}

// isOverdue reports whether an unresolved ticket has passed its due date.
func isOverdue(t *Ticket, now time.Time) bool {
	if t.DueBy == "" || t.Status >= statusResolved {
		return false
	}
	dueBy, err := time.Parse(time.RFC3339, t.DueBy)
	if err != nil {
		return false
	}
	return now.After(dueBy)
}
