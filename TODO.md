# Required

 * "follow outline" of gcode file
 * better ui for pause/resume etc.
 * unlock an alarm
 * show grbl's messages to user (alarm/error/other message)
 * overrides
 * fix keyboard jog
 * make keyboard shortcuts more sensible
 * fix deadlocks

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

# Refactors

 * Numpop & MDI duplicate a lot of code to do with input boxes
 * I don't like all the `if axis == "X" { ... } else if axis == "Y" { ... }` etc. stuff for handling the X/Y/Z/A DRO, should make that neater
 * The jog status has editable fields like the DRO, but they want to be formatted differently, I should abstract this into an "editable number" component, that uses numpop underneath for the actual editing, but gives more control over formatting, and then `LayoutDROCoord` and `LayoutJogField` or something can both use it
 * I don't like the global `entered` map for the mouseover in the editable DRO
 * I don't like the global `list` and `scrolledTo` variables in the gcode view
