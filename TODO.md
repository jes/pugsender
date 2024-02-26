# Required

 * better ui for pause/resume etc.
 * show grbl's messages to user (alarm/error/other message)
 * fix keyboard jog
 * make keyboard shortcuts more sensible
 * make a cheatsheet of keyboard shortcuts
 * fix deadlocks

# Nice-to-have

## Serial connection

 * allow disabling autoconnect
 * allow connecting to manually-typed serial port
 * raw serial console
 * better indication of auto-connector behaviour (specifically, when it doesn't connect: why? and what did it try?)
 * only try to connect to ports that have newly-appeared (or disappeared and reappeared) since last connection attempt?

## Gcode files

 * show key shortcuts & mouse operations
 * cycle time estimate
 * gcode previews in file selector
 * show gcode filename in status bar
 * detect USB stick insertion & suggest to open the last-modified file, if nothing is currently open
 * "follow outline" of gcode file
 * preprocess gcode to minimise the number of bytes to send over the wire, canonicalise case, strip unnecessary digits, skip comments and blank lines, etc.

## Gcode lines display
 * "run from line", "run to line", on gcode view
 * "pause after line" on gcode view?
 * allow editing gcode?
 * highlight gcode lines in path view when hovered in text view
 * search in gcode view (e.g. to find "Begin profile" comments or whatever)
 * line numbers
 * colour the lines based on: not sent yet, waiting in serial buffer, waiting in planner buffer, currently executing, completed
 * right-click for context menu?

## Interface

 * efficient file browser, keyboard-first, tab-completion
 * mouse interface
 * some sort of "processing" indicator while rendering toolpaths, loading gcode, etc.
 * correctly handle the distinction between pixels, Dp, Sp, etc.?
 * when window is full-screen, file selector should be visible, instead of opened "behind" the application
 * make the user confirm if they try to close while program is running
 * support 4th axis (e.g. with grblHAL)
 * icons on buttons
 * make the relevant button show the click animation when you press the keyboard shortcut
 * stash reference coordinates, and then "set work offset" or "jog to here" on them, allow saving XY only or XYZ
 * keyboard shortcuts for overrides
 * stash override settings in some "register", and restore/revert on some macro keypress (maybe make this even more powerful, so it can do other things too)
 * click on "Alarm" label to unlock (maybe also overlay "click to unlock")
 * if jog inc/feed numpop would overflow bottom of screen, draw it further up
 * more levels of undo? undo more things?
 * pop up a message saying you can ctrl-z to undo after assigning to undoWco (more generally, some way to popup messages)
 * different UI for rapid override since it only supports 25%, 50%, 100%?

## MDI

 * history search
 * history view
 * make the MDI all caps, automatically insert spaces between a number and a following letter (so "G0x0y0" becomes "G0 X0 Y0")
 * prevent submitting the MDI if there is nothing to submit to? (e.g. disconnected)
 * make the MDI detect all arrow key events (why are some suppressed?)
 * fix history scrolling
 * if you scroll all the way to bottom of history, then restore whatever un-sent half-typed command was already there (may be empty)

## 2d view

 * reset 2d view
 * don't draw lines on toolpath view when changing work offset
 * zoom toolpath view to fit content
 * don't draw an initial movement from (0,0,0) to the start point of the g-code
 * show gcode bounding box dimensions in toolpath view
 * make it so the colours in the toolpath view can distinguish between "machine has moved here", "machine will move here", and "machine has moved here and will move here again"
 * simplify paths (for both gcode path and actual-movement path) so that points that lie on a straight line through the previous 2 points are updated in-place instead of adding a new point
 * right-click for context menu?

## Saving state

 * save settings to file
 * remember config: jog inc, jog feed, jog rapid, split layout ratios
 * stash grbl config and allow restore (and save to file?)
 * zoom level
 * is there a sensible config lib to use, instead of manual formatting/parsing?
 * MDI history

## Gcode sending

 * drain on tool change, and allow touching off
 * make a new GCodeRunner on gcode load, and destroy the old one, so that there is no data race
 * on CmdStop, send a feed hold and wait for status "Hold:2" before sending soft-reset
 * detect errors from respChan, probably feed hold, and alert the user
 * use character-counting instead of waiting for a response before sending the next line?
 * stop requesting G codes after every command (but how else do you display up-to-date G codes?)
 * In `GCodeRunner.Path()`, only update `pos` for commands that are actually movements
 * In `GCodeRunner.Path()`, handle G2, G3, etc.
 * In `Grbl.Run()`, if the command to write implies eeprom access, then block until it is complete, don't write any more yet, because https://github.com/gnea/grbl/wiki/Grbl-v1.1-Interface#eeprom-issues - and then make `SetWpos` stop using `CommandWait`
