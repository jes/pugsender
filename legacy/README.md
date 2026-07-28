# pugsender

**G-code sender that gets out of the way. Work-in-progress.**

The GUI will be mostly keyboard-driven and aims to be frustration-free.

I would rather leave the MDI activated and trust the user not to abuse it, than
make the user wait before they're allowed to type in the MDI like UGS does. In particular I want to allow MDI usage while a job is running (!) even though it will go at a "random" point in the stream, if for no other reason than to allow sending `M3 S5000` if I forgot to put that in the program. [done]

I want keyboard jogging to work as well as it would in a video game, and I want
to change jog increment and feed rate without breaking the flow.

I want to set the work offset with no clicking and a minimum of keystrokes. [done]

I want it to find a Grbl and connect automatically, instead of making me guess
the name of the serial device like UGS does. [done]

I want it to remember the coordinate system across a disconnect or restart. [done]

I want it to indicate the probe pin state in the GUI [done]

I want a frustration-free file selector, keyboard-first & with tab-completion.

I want previews of G-code files in the file selector.

I want accurate cycle time estimates (i.e. preprocess the G-code and take the distances and feed rates into
account, not just the number of lines).

I don't ever want g-code processing or toolpath rendering to block the GUI (if it is taking a long
time to render something then *that thing* can get laggy, but the rest of the GUI should run at full
speed). [done]

Installing dependencies (Fedora 39):

  $ sudo dnf install golang libX11-devel libXcursor-devel libXfixes-devel libwayland-client-devel libxkbcommon-devel libxkbcommon-x11-devel mesa-libEGL-devel vulkan-headers wayland-devel
