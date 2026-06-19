package server

import (
	"sync"
	"time"
)

type approvalRateLimiters struct {
	mu sync.Mutex

	startMinute map[string]approvalRateWindow
	startHour   map[string]approvalRateWindow
	approveMin  map[string]approvalRateWindow
	approveHour map[string]approvalRateWindow
	statusMin   map[string]approvalRateWindow
	statusIPMin map[string]approvalRateWindow
	claimMin    map[string]approvalRateWindow
	claimIPMin  map[string]approvalRateWindow
}

type approvalRateWindow struct {
	start time.Time
	count int
}

func newApprovalRateLimiters() *approvalRateLimiters {
	return &approvalRateLimiters{
		startMinute: map[string]approvalRateWindow{},
		startHour:   map[string]approvalRateWindow{},
		approveMin:  map[string]approvalRateWindow{},
		approveHour: map[string]approvalRateWindow{},
		statusMin:   map[string]approvalRateWindow{},
		statusIPMin: map[string]approvalRateWindow{},
		claimMin:    map[string]approvalRateWindow{},
		claimIPMin:  map[string]approvalRateWindow{},
	}
}

func (l *approvalRateLimiters) allowApprovalStart(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return allowApprovalRate(l.startMinute, ip, now, 10, time.Minute) &&
		allowApprovalRate(l.startHour, ip, now, 30, time.Hour)
}

func (l *approvalRateLimiters) allowApprovalApprove(session string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return allowApprovalRate(l.approveMin, session, now, 20, time.Minute) &&
		allowApprovalRate(l.approveHour, session, now, 60, time.Hour)
}

func (l *approvalRateLimiters) allowApprovalStatus(session string, ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return allowApprovalRate(l.statusMin, session, now, 120, time.Minute) &&
		allowApprovalRate(l.statusIPMin, ip, now, 240, time.Minute)
}

func (l *approvalRateLimiters) allowApprovalClaim(approvalID string, ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return allowApprovalRate(l.claimMin, approvalID, now, 90, time.Minute) &&
		allowApprovalRate(l.claimIPMin, ip, now, 120, time.Minute)
}

func allowApprovalRate(windows map[string]approvalRateWindow, key string, now time.Time, maxCount int, window time.Duration) bool {
	start := now.Truncate(window)
	current := windows[key]
	if current.start != start {
		windows[key] = approvalRateWindow{start: start, count: 1}
		return true
	}
	if current.count >= maxCount {
		return false
	}
	current.count++
	windows[key] = current
	return true
}
