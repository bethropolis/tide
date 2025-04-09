package modehandler

import (
	"github.com/bethropolis/tide/internal/logger"
)

// stopLeaderTimer safely stops the timer.
func (mh *ModeHandler) stopLeaderTimer() {
	if mh.leaderTimer != nil {
		mh.leaderTimer.Stop()
		mh.leaderTimer = nil
	}
}

// resetLeaderState clears the waiting state and timer.
func (mh *ModeHandler) resetLeaderState() {
	if mh.leaderWaiting {
		logger.Debugf("Resetting leader state")
		mh.leaderWaiting = false
		mh.stopLeaderTimer()
	}
}
