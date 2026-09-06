package wire

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/update"
)

// stages maps this app's stages onto the contract's.
//
// A table rather than a switch, so a stage added without a wire value fails at the conversion
// rather than crossing as unspecified.
var stages = map[update.Stage]screensharev1.UpdateStage{
	update.StageOff:       screensharev1.UpdateStage_UPDATE_STAGE_OFF,
	update.StageUnchecked: screensharev1.UpdateStage_UPDATE_STAGE_UNCHECKED,
	update.StageChecking:  screensharev1.UpdateStage_UPDATE_STAGE_CHECKING,
	update.StageCurrent:   screensharev1.UpdateStage_UPDATE_STAGE_CURRENT,
	update.StageAvailable: screensharev1.UpdateStage_UPDATE_STAGE_AVAILABLE,
	update.StageFetching:  screensharev1.UpdateStage_UPDATE_STAGE_FETCHING,
	update.StageReady:     screensharev1.UpdateStage_UPDATE_STAGE_READY,
	update.StageFailed:    screensharev1.UpdateStage_UPDATE_STAGE_FAILED,
}

// UpdateState converts what this install knows about the published release.
func UpdateState(state update.State) *screensharev1.UpdateState {
	stage, declared := stages[state.Stage]
	assert.Assert(declared, "every stage an install reaches crosses the contract", int(state.Stage))
	assert.Assert(state.Percent >= 0 && state.Percent <= 100,
		"a download crosses as a fraction of itself", state.Percent)

	return &screensharev1.UpdateState{
		Stage:         stage,
		Running:       state.Running,
		Latest:        state.Latest,
		Page:          state.Page,
		Percent:       int32(state.Percent),
		Unchecked:     state.Unchecked,
		Uninstallable: state.Uninstallable,
		Failure:       state.Failure,
		Detail:        state.Detail,
	}
}

// UpdateStateEvent announces what this install knows about the published release, whole.
//
// On every step of a check and as a download fills in,
// so a second window learns that a release is staged without having asked for one.
func UpdateStateEvent(state update.State) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_UpdateState{UpdateState: UpdateState(state)},
	}
}
