package main

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	v1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// Fields whose value reaches something outside this process, so a probe leaves them where the run
// set them.
var frozen = map[string]bool{
	"relay.host":           true,
	"relay.group_key":      true,
	"relay.srt_passphrase": true,
	"publish.name":         true,
}

// runForm walks the settings space one legal move at a time and checks what the resolver owes.
//
// One field per resolve is what makes the checks say anything: the option was enabled on the form
// the draft came from, so a repair landing on it is a contradiction rather than a consequence of
// something else having moved.
func runForm(ctx context.Context, run *session, rng *rand.Rand, until time.Time) error {
	settings, err := run.settled(ctx)
	if err != nil {
		return err
	}

	form, err := run.resolve(ctx, settings)
	if err != nil {
		return err
	}
	settings = form.GetSettings()

	for iteration := 0; time.Now().Before(until); iteration++ {
		if ctx.Err() != nil {
			return nil
		}
		run.report.setIteration(iteration)
		run.report.progress("")

		checkForm(run, form, settings)

		// Back to the stored draft now and then, so a walk that reached a corner of the space
		// leaves it instead of resolving there for the rest of the run.
		if iteration%200 == 199 {
			settings, err = run.settled(ctx)
			if err != nil {
				return err
			}
			form, err = run.resolve(ctx, settings)
			if err != nil {
				return err
			}
			settings = form.GetSettings()
			continue
		}

		moves := mutables(form, frozen)
		if len(moves) == 0 {
			run.report.report("form.no_move", "form/no-enabled-field",
				"no visible, enabled field offers a value", nil, settings)
			settings, _ = run.settled(ctx)
			form, err = run.resolve(ctx, settings)
			if err != nil {
				return err
			}
			settings = form.GetSettings()
			continue
		}

		move := moves[rng.Intn(len(moves))]
		draft := proto.Clone(settings).(*v1.Settings)
		chosen, err := mutate(rng, draft, move)
		if err != nil {
			run.report.report("form.unwritable_key", "form/key/"+move.key,
				err.Error(), map[string]string{"key": move.key}, settings)
			continue
		}

		next, err := run.resolve(ctx, draft)
		if err != nil {
			run.report.report("rpc.resolve_failed", "rpc/resolve/"+move.key,
				err.Error(), map[string]string{"key": move.key, "value": chosen}, draft)
			if ctx.Err() != nil {
				return nil
			}
			settings, _ = run.settled(ctx)
			form, _ = run.resolve(ctx, settings)
			continue
		}

		checkMove(run, move, chosen, draft, next)

		// The fixpoint: what a resolve handed back is what the next one has nothing to repair in.
		settled, err := run.resolve(ctx, next.GetSettings())
		if err == nil {
			if keys := settled.GetRepairedFieldKeys(); len(keys) > 0 {
				run.report.report("form.repair_not_settled", "form/unsettled/"+keys[0],
					fmt.Sprintf("a resolve repaired %v in the settings a resolve had just returned", keys),
					map[string]string{"moved": move.key, "value": chosen}, next.GetSettings())
			}
			if !proto.Equal(settled.GetSettings(), next.GetSettings()) {
				run.report.report("form.resolve_not_stable", "form/unstable/"+move.key,
					"resolving a returned draft returned a different draft",
					map[string]string{"moved": move.key, "value": chosen}, next.GetSettings())
			}
		}

		// Same draft twice, same screen: a resolve is a read and computes nothing it keeps.
		// Every few iterations, the answer being the same one for as long as nothing else moved.
		if iteration%7 == 0 {
			again, err := run.resolve(ctx, draft)
			if err == nil && !proto.Equal(again, next) {
				where := formDiff(next, again)
				run.report.report("form.resolve_not_pure", "form/impure/"+where,
					"two resolves of one draft described the screen differently at "+where,
					map[string]string{"moved": move.key, "value": chosen, "differs": where}, draft)
			}
		}

		form, settings = next, next.GetSettings()
	}
	return nil
}

// checkMove holds a resolve to what the form it came from promised.
func checkMove(run *session, move mutable, chosen string, draft *v1.Settings, next *v1.Form) {
	for _, key := range next.GetRepairedFieldKeys() {
		if key != move.key {
			continue
		}
		// An entry taken off a list is what its own removal entry does, so the key naming it is
		// gone rather than walked.
		if _, _, err := locate(next.GetSettings(), move.key, false); err != nil {
			continue
		}
		run.report.report("form.enabled_option_repaired", "form/repaired/"+move.key+"="+chosen,
			fmt.Sprintf("%s was offered as enabled and the resolve walked it to %q",
				move.key, readField(next.GetSettings(), move.key)),
			map[string]string{"key": move.key, "wrote": chosen, "became": readField(next.GetSettings(), move.key)},
			draft)
	}
	run.report.pass()
}

// checkForm states what every form owes whatever draft produced it.
func checkForm(run *session, form *v1.Form, settings *v1.Settings) {
	for _, group := range form.GetGroups() {
		for _, field := range group.GetFields() {
			if !field.GetVisible() {
				continue
			}
			key := field.GetKey()

			switch field.GetControl() {
			case v1.ControlKind_CONTROL_KIND_SELECT, v1.ControlKind_CONTROL_KIND_RADIO, v1.ControlKind_CONTROL_KIND_NUMBER_SELECT:
				enabled := 0
				held := false
				value := readField(settings, key)
				for _, option := range field.GetOptions() {
					if option.GetEnabled() {
						enabled++
					}
					if option.GetValue() == value {
						held = true
					}
				}
				if field.GetEnabled() && enabled == 0 {
					run.report.report("form.select_without_options", "form/empty-select/"+key,
						"an enabled control offers no value that can be chosen",
						map[string]string{"key": key}, settings)
				}
				// A number-select accepts its range, the entries being shortcuts rather than the domain,
				// so a held value off the ladder is ordinary there
				// (api/proto/screenshare/v1/form.proto, CONTROL_KIND_NUMBER_SELECT).
				offers := field.GetControl() != v1.ControlKind_CONTROL_KIND_NUMBER_SELECT
				if offers && len(field.GetOptions()) > 0 && !held && value != "" {
					run.report.report("form.value_not_offered", "form/unlisted-value/"+key,
						fmt.Sprintf("the control holds %q and offers no such entry", value),
						map[string]string{"key": key, "value": value}, settings)
				}
				for _, option := range field.GetOptions() {
					if !option.GetEnabled() && option.GetReason().GetCode() == v1.TextCode_TEXT_CODE_UNSPECIFIED {
						run.report.report("form.greying_without_reason", "form/no-reason/"+key+"/"+option.GetValue(),
							"a greyed entry names no reason",
							map[string]string{"key": key, "option": option.GetValue()}, settings)
					}
				}
			case v1.ControlKind_CONTROL_KIND_NUMBER, v1.ControlKind_CONTROL_KIND_SLIDER:
				r := field.GetRange()
				if field.GetEnabled() && r != nil && r.GetMax() < r.GetMin() {
					run.report.report("form.empty_range", "form/empty-range/"+key,
						fmt.Sprintf("range is %d..%d", r.GetMin(), r.GetMax()),
						map[string]string{"key": key}, settings)
				}
			}

			if !field.GetEnabled() && field.GetReason().GetCode() == v1.TextCode_TEXT_CODE_UNSPECIFIED {
				run.report.report("form.disabled_without_reason", "form/no-reason/"+key,
					"a greyed field names no reason",
					map[string]string{"key": key}, settings)
			}
		}
	}

	summary := form.GetSummary()
	if form.GetPublishable() {
		if summary.GetCommand() == "" {
			run.report.report("form.publishable_without_command", "form/no-command",
				"the form says the draft can publish and renders no command", nil, settings)
		}
		if summary.GetCommandError() != "" {
			run.report.report("form.publishable_with_command_error", "form/command-error/"+summary.GetCommandError(),
				summary.GetCommandError(), nil, settings)
		}
	}
	run.report.pass()
}

// formDiff names where two descriptions of one draft stop agreeing, so an impurity is triaged from
// the finding rather than from a rerun.
func formDiff(a, b *v1.Form) string {
	if a.GetPublishable() != b.GetPublishable() {
		return "publishable"
	}
	if strings.Join(a.GetRepairedFieldKeys(), ",") != strings.Join(b.GetRepairedFieldKeys(), ",") {
		return "repaired_field_keys"
	}
	if !proto.Equal(a.GetSettings(), b.GetSettings()) {
		return "settings"
	}
	if !proto.Equal(a.GetSummary(), b.GetSummary()) {
		switch {
		case a.GetSummary().GetCommand() != b.GetSummary().GetCommand():
			return "summary.command"
		case !proto.Equal(a.GetSummary().GetEstimate(), b.GetSummary().GetEstimate()):
			return "summary.estimate"
		default:
			return "summary"
		}
	}
	if len(a.GetDiagnostics()) != len(b.GetDiagnostics()) {
		return "diagnostics"
	}
	for i := range a.GetDiagnostics() {
		if !proto.Equal(a.GetDiagnostics()[i], b.GetDiagnostics()[i]) {
			return "diagnostics." + a.GetDiagnostics()[i].GetFieldKey()
		}
	}
	for i, group := range a.GetGroups() {
		if i >= len(b.GetGroups()) {
			return "groups.count"
		}
		other := b.GetGroups()[i]
		for j, field := range group.GetFields() {
			if j >= len(other.GetFields()) {
				return group.GetKey() + ".field-count"
			}
			mirror := other.GetFields()[j]
			if field.GetKey() != mirror.GetKey() {
				return group.GetKey() + ".field-order"
			}
			if proto.Equal(field, mirror) {
				continue
			}
			switch {
			case field.GetEnabled() != mirror.GetEnabled():
				return field.GetKey() + ".enabled"
			case field.GetVisible() != mirror.GetVisible():
				return field.GetKey() + ".visible"
			case !proto.Equal(field.GetValue(), mirror.GetValue()):
				return field.GetKey() + ".value"
			case len(field.GetOptions()) != len(mirror.GetOptions()):
				return field.GetKey() + ".option-count"
			}
			for k, option := range field.GetOptions() {
				if !proto.Equal(option, mirror.GetOptions()[k]) {
					return field.GetKey() + ".option." + option.GetValue()
				}
			}
			return field.GetKey()
		}
	}
	if len(a.GetPresets()) != len(b.GetPresets()) {
		return "presets.count"
	}
	for i, preset := range a.GetPresets() {
		if !proto.Equal(preset, b.GetPresets()[i]) {
			return "presets." + preset.GetKey()
		}
	}
	return "elsewhere"
}
