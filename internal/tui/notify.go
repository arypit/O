package tui

import "time"

// notification is one high-priority event shown in the notification area.
// Routine activity never creates one.
type notification struct {
	id    int
	level string // info | success | warn | error
	title string
	body  string
	at    time.Time
}

// notify adds a notification to the notification area and to the history.
// Notifications are only used for events that need attention.
func (m *Model) notify(level, title, body string) {
	if !m.notificationsOn {
		return
	}
	m.noteSeq++
	n := notification{id: m.noteSeq, level: level, title: title, body: body, at: time.Now()}
	m.notes = append(m.notes, n)
	if len(m.notes) > 4 {
		m.notes = m.notes[len(m.notes)-4:]
	}
	m.noteHistory = append(m.noteHistory, n)
	if len(m.noteHistory) > 50 {
		m.noteHistory = m.noteHistory[len(m.noteHistory)-50:]
	}
}

// expireNotifications removes notifications that have been visible long
// enough. A duration of zero keeps them until they are replaced.
func (m *Model) expireNotifications() {
	if m.noteTTLSeconds <= 0 || len(m.notes) == 0 {
		return
	}
	ttl := time.Duration(m.noteTTLSeconds) * time.Second
	cutoff := time.Now().Add(-ttl)
	kept := m.notes[:0]
	for _, n := range m.notes {
		if n.at.After(cutoff) {
			kept = append(kept, n)
		}
	}
	m.notes = kept
}

// dismissNotification removes one notification from the area.
func (m *Model) dismissNotification(id int) {
	kept := m.notes[:0]
	for _, n := range m.notes {
		if n.id != id {
			kept = append(kept, n)
		}
	}
	m.notes = kept
}
