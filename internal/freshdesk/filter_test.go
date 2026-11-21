package freshdesk

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func makeTicket(id int64, subject, ticketType string, status, priority int) Ticket {
	return Ticket{
		ID:       id,
		Subject:  subject,
		Type:     ticketType,
		Status:   status,
		Priority: priority,
	}
}

func TestFilterTickets_NoFilter(t *testing.T) {
	tickets := []Ticket{
		makeTicket(1, "network issue", "Problem", 2, 1),
		makeTicket(2, "billing question", "Question", 3, 2),
	}
	result := filterTickets(tickets, &TicketFilter{})
	assert.Len(t, result, 2)
}

func TestFilterTickets_ByQuery_Subject(t *testing.T) {
	tickets := []Ticket{
		makeTicket(1, "network issue", "Problem", 2, 1),
		makeTicket(2, "billing question", "Question", 3, 2),
	}
	result := filterTickets(tickets, &TicketFilter{Query: "network"})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
}

func TestFilterTickets_ByQuery_Type(t *testing.T) {
	tickets := []Ticket{
		makeTicket(1, "some subject", "Problem", 2, 1),
		makeTicket(2, "other subject", "Question", 3, 2),
	}
	result := filterTickets(tickets, &TicketFilter{Query: "problem"})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
}

func TestFilterTickets_ByStatus(t *testing.T) {
	tickets := []Ticket{
		makeTicket(1, "open ticket", "", 2, 1),
		makeTicket(2, "resolved ticket", "", 4, 1),
	}
	result := filterTickets(tickets, &TicketFilter{Status: 2})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
}

func TestFilterTickets_ByPriority(t *testing.T) {
	tickets := []Ticket{
		makeTicket(1, "low priority", "", 2, 1),
		makeTicket(2, "high priority", "", 2, 3),
	}
	result := filterTickets(tickets, &TicketFilter{Priority: 3})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(2), result[0].ID)
}

func TestFilterTickets_ByType(t *testing.T) {
	tickets := []Ticket{
		makeTicket(1, "a", "Problem", 2, 1),
		makeTicket(2, "b", "Question", 2, 1),
	}
	result := filterTickets(tickets, &TicketFilter{Type: "problem"})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
}

func TestFilterTickets_ByType_CaseInsensitive(t *testing.T) {
	tickets := []Ticket{
		makeTicket(1, "a", "PROBLEM", 2, 1),
	}
	result := filterTickets(tickets, &TicketFilter{Type: "problem"})
	assert.Len(t, result, 1)
}

func TestFilterTickets_ByEscalated(t *testing.T) {
	tickets := []Ticket{
		{ID: 1, IsEscalated: true},
		{ID: 2, IsEscalated: false},
	}
	result := filterTickets(tickets, &TicketFilter{IsEscalated: true})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
}

func TestFilterTickets_ByRequesterID(t *testing.T) {
	tickets := []Ticket{
		{ID: 1, RequesterID: 100},
		{ID: 2, RequesterID: 200},
	}
	result := filterTickets(tickets, &TicketFilter{RequesterID: 100})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
}

func TestFilterTickets_ByCompanyID(t *testing.T) {
	tickets := []Ticket{
		{ID: 1, CompanyID: 10},
		{ID: 2, CompanyID: 20},
	}
	result := filterTickets(tickets, &TicketFilter{CompanyID: 10})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
}

func TestFilterTickets_ByGroupID(t *testing.T) {
	tickets := []Ticket{
		{ID: 1, GroupID: 5},
		{ID: 2, GroupID: 6},
	}
	result := filterTickets(tickets, &TicketFilter{GroupID: 5})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
}

func TestFilterTickets_ByCreatedAfter(t *testing.T) {
	tickets := []Ticket{
		{ID: 1, CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: 2, CreatedAt: "2024-06-01T00:00:00Z"},
	}
	result := filterTickets(tickets, &TicketFilter{CreatedAfter: "2024-03-01T00:00:00Z"})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(2), result[0].ID)
}

func TestFilterTickets_ByCreatedBefore(t *testing.T) {
	tickets := []Ticket{
		{ID: 1, CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: 2, CreatedAt: "2024-06-01T00:00:00Z"},
	}
	result := filterTickets(tickets, &TicketFilter{CreatedBefore: "2024-03-01T00:00:00Z"})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
}

func TestFilterTickets_Overdue(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	tickets := []Ticket{
		{ID: 1, DueBy: past, Status: 2},   // overdue
		{ID: 2, DueBy: future, Status: 2}, // not yet due
		{ID: 3, DueBy: past, Status: 4},   // resolved - not overdue
		{ID: 4, DueBy: "", Status: 2},     // no due date
	}
	result := filterTickets(tickets, &TicketFilter{Overdue: true})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
}

func TestFilterTickets_MultipleFilters(t *testing.T) {
	tickets := []Ticket{
		{ID: 1, Subject: "network issue", Status: 2, Priority: 3, CompanyID: 10},
		{ID: 2, Subject: "network issue", Status: 2, Priority: 1, CompanyID: 10},
		{ID: 3, Subject: "billing", Status: 2, Priority: 3, CompanyID: 10},
	}
	result := filterTickets(tickets, &TicketFilter{
		Query:     "network",
		Priority:  3,
		CompanyID: 10,
	})
	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
}

func TestFilterTickets_EmptyInput(t *testing.T) {
	result := filterTickets([]Ticket{}, &TicketFilter{Query: "anything"})
	assert.Empty(t, result)
}

func TestFilterTickets_ByAgentID(t *testing.T) {
	tickets := []Ticket{
		{ID: 1, ResponderID: 100},
		{ID: 2, ResponderID: 200},
		{ID: 3, ResponderID: 100},
	}

	result := filterTickets(tickets, &TicketFilter{AgentID: 100})

	assert.Len(t, result, 2)
	assert.Equal(t, int64(1), result[0].ID)
	assert.Equal(t, int64(3), result[1].ID)
}

func TestFilterTickets_ByAgentIDAndStatus(t *testing.T) {
	tickets := []Ticket{
		{ID: 1, ResponderID: 100, Status: 2},
		{ID: 2, ResponderID: 100, Status: 4},
		{ID: 3, ResponderID: 200, Status: 2},
	}

	result := filterTickets(tickets, &TicketFilter{AgentID: 100, Status: 2})

	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].ID)
}
