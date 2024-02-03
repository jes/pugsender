# Required

 * "follow outline" of gcode file
 * better ui for pause/resume etc.
 * show grbl's messages to user (alarm/error/other message)
 * overrides
 * fix keyboard jog
 * make keyboard shortcuts more sensible
 * fix deadlocks
 * if jog inc/feed numpop would overflow bottom of screen, draw it further up

# Nice-to-have

 * allow disabling autoconnect
 * allow connecting to manually-typed serial port
 * efficient file browser, keyboard-first, tab-completion
 * show key shortcuts & mouse operations
 * raw serial console
 * mouse interface
 * better indication of auto-connector behaviour (specifically, when it doesn't connect: why? and what did it try?)
 * don't attempt jogging while mode is not "idle" or "jog"?
 * MDI history search
 * MDI history view
 * make the MDI all caps, automatically insert spaces between a number and a following letter (so "G0x0y0" becomes "G0 X0 Y0")
 * reset toolpath view
 * save settings to file
 * don't draw lines on toolpath view when changing work offset
 * zoom toolpath view to fit content
 * prevent submitting the MDI if there is nothing to submit to? (e.g. disconnected)
 * make the MDI detect all arrow key events (why are some suppressed?)
 * some sort of "processing" indicator while rendering toolpaths, loading gcode, etc.
 * "run from line" on gcode view
 * "pause after line" on gcode view?
 * allow editing gcode?
 * cycle time estimate
 * gcode previews in file selector
 * printable cheatsheet showing keyboard controls
 * correctly handle the distinction between pixels, Dp, Sp, etc.?
 * don't draw an initial movement from (0,0,0) to the start point of the g-code
 * when window is full-screen, file selector should be visible, instead of opened "behind" the application
 * show gcode filename in status bar
 * make the user confirm if they try to close while program is running
 * support 4th axis (e.g. with grblHAL)
