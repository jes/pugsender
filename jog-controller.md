# Jog controller

The jog controller combines fixed steps for key taps with continuous
movement for held keys. It tracks destinations locally and sends a stream of
short Grbl jog commands.

The main code is in [jog.go](jog.go), with per-axis state in
[jogaxis.go](jogaxis.go). Keyboard events are handled in [app.go](app.go).

## Usage

Keyboard jogging is available in JOG mode while the controller reports `Idle`
or `Jog`.

| Key | Movement |
| --- | --- |
| Left / Right | X negative / positive |
| Down / Up | Y negative / positive |
| Page Down / Page Up | Z negative / positive |
| Shift, while jogging | Use the faster jog feed rate |

Tap a direction key to move by the configured increment, which defaults to 1.
Hold a direction key for continuous movement. Releasing a held direction key
cancels its queued movement; releasing a simple tap allows its step to finish.

**Hold Shift while jogging to go faster.** Pressing Shift switches the active
feed from the normal `FeedRate` (default 100 units/minute) to `RapidFeedRate`
(default 1000 units/minute). Releasing Shift restores the normal feed. With the
defaults, Shift selects a feed rate ten times higher. A speed change cancels
the existing jog and resends any remaining movement at the new feed rate.
The faster setting is still a `$J=` jog with an explicit feed, rather than a
`G0` rapid command.

In JOG mode, `I` opens the increment editor, `F` opens the normal feed editor,
and `P` opens the faster feed editor. These editors change the configured
values; the active feed is selected when Shift is pressed or released.

Ctrl-clicking the toolpath requests a jog to the clicked X/Y work coordinates
through `JogTo`.

## Input and targets

`JogControl.Update` translates direction keys into axis changes:

- A press adds or subtracts `Increment` from the axis's incremental target.
- A hold sets a positive or negative direction flag.
- A release clears the corresponding held flag and requests cancellation if
  that direction was held.

Each axis maintains two destinations:

- `IncrementalTarget` accumulates discrete key presses.
- `Target` is the destination sent to Grbl, including extensions from holding
  a key.

For example, starting at X=10 with an increment of 1, a right-arrow tap requests
X=11. Holding right then advances `Target` beyond 11, while
`IncrementalTarget` stays at 11.

This separation helps combine tapping and holding without pulling the machine
back toward an old step destination. In `SetIncrementalTarget`, an idle axis
or an axis with an unfinished incremental target adopts the new incremental
target directly. Otherwise, the new incremental target replaces the moving
target only if it extends farther in the same direction, or the moving target
is effectively at the current position.

## The 100 ms update loop

`JogControl.Run` runs in a background goroutine. Every 100 ms it:

1. Updates each axis from the reported work position and estimated velocity.
2. Extends held-axis targets by
   `ActiveFeedRate * TickerPeriod.Minutes()`.
3. Sends targets marked `NeedsCommand`.

At the default normal feed of 100 units/minute, a held axis advances its target
by about 0.167 units per tick. With Shift selecting the default faster feed of
1000 units/minute, that becomes about 1.667 units per tick.

Multiple axes can appear in one command, for example:

```text
$J=X11.167Y5.167F100.000
```

Only axes marked `NeedsCommand` appear in the command. `SendJog` sends only
while the controller reports `Idle` or `Jog`, and `SendJogCommand` requires at
least two free planner slots. The serial layer also checks readiness and
available serial buffer space.

Successfully queuing a command clears the pending flags. If queuing fails,
the flags remain available for a later tick. The controller's eventual command
response is consumed without being used to update jog state, so queue success
does not confirm that Grbl accepted or completed the movement.

## Cancellation and continuation

New step presses, held-key releases, and active feed changes request jog
cancellation. If a jog has been queued since the last cancellation, `Cancel`
sends realtime byte `0x85` and calls `WasCancelled` on every axis.

Cancellation preserves local targets. Unfinished incremental moves and
still-held axes are marked for resending. This allows one axis to continue
after another is released, and allows remaining movement to resume at a new
feed rate when Shift is pressed or released.

Keyboard updates normally wait for the next 100 ms tick to send their revised
targets. `SetActiveFeedRate` and `JogTo` also attempt to send immediately after
cancelling.

## Position feedback and idle reset

`JogAxis.Update` considers an incremental target reached when the reported
position crosses it or comes within 0.0001 units of it.

When an axis has no held direction keys and has spent roughly 500 ms without
motion or a target update, both destinations reset to its reported position.
The axis becomes idle and its pending command flag clears. The next tap then
starts from the actual position.

Grbl status is requested every 200 ms. The serial layer estimates axis
velocity from the change in machine position between status reports; that
estimate is used to detect motion for the idle reset.

## Prototype limitations

- Hold detection depends on receiving a release followed by a press for the
  same key within one GUI frame. That sequence becomes `JogKeyHold`; there is
  no elapsed-time hold detector. The code itself questions this assumption,
  and `TODO.md` lists fixing keyboard jogging as required work.
- Commands contain absolute-style work-coordinate targets but do not
  explicitly specify `G90` or `G21`. Their interpretation therefore depends
  on the controller's current distance and unit modes.
- Four-axis state exists, but the A-axis direction keys are placeholders.
- The background loop and GUI handlers share mutable jog state without
  explicit synchronization.
