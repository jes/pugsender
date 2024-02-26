# Required

 * better ui for pause/resume etc.
 * show grbl's messages to user (alarm/error/other message)
 * fix keyboard jog
 * make keyboard shortcuts more sensible
 * make a cheatsheet of keyboard shortcuts
 * fix deadlocks
 * if jog inc/feed numpop would overflow bottom of screen, draw it further up

# Nice-to-have

## Serial connection

 * allow disabling autoconnect
 * allow connecting to manually-typed serial port
 * raw serial console
 * better indication of auto-connector behaviour (specifically, when it doesn't connect: why? and what did it try?)

## Gcode files

 * show key shortcuts & mouse operations
 * "run from line", "run to line", on gcode view
 * "pause after line" on gcode view?
 * allow editing gcode?
 * cycle time estimate
 * gcode previews in file selector
 * show gcode filename in status bar
 * line numbers on gcode view
 * detect USB stick insertion & suggest to open the last-modified file, if nothing is currently open
 * "follow outline" of gcode file
 * highlight gcode lines in path view when hovered in text view
 * search in gcode view (e.g. to find "Begin profile" comments or whatever)
 * line numbers
 * preprocess gcode to minimise the number of bytes to send over the wire, canonicalise case, strip unnecessary digits, skip comments and blank lines, etc.

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

## MDI

 * history search
 * history view
 * make the MDI all caps, automatically insert spaces between a number and a following letter (so "G0x0y0" becomes "G0 X0 Y0")
 * prevent submitting the MDI if there is nothing to submit to? (e.g. disconnected)
 * make the MDI detect all arrow key events (why are some suppressed?)

## 2d view

 * reset 2d view
 * don't draw lines on toolpath view when changing work offset
 * zoom toolpath view to fit content
 * don't draw an initial movement from (0,0,0) to the start point of the g-code
 * show gcode bounding box dimensions in toolpath view
 * make it so the colours in the toolpath view can distinguish between "machine has moved here", "machine will move here", and "machine has moved here and will move here again"

## Saving state

 * save settings to file
 * remember config: jog inc, jog feed, jog rapid, split layout ratios
 * stash grbl config and allow restore (and save to file?)

## Gcode sending

 * drain on tool change, and allow touching off

