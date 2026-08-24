Acknowledges every currently active alarm at once instead of clicking rows individually. Acknowledgement changes the alarm's visual state - flashing stops, the row dims - but does nothing about the underlying condition: the alarm clears only when its process variable returns to normal, and it reactivates if the condition returns before being cleared.

The annunciator philosophy follows the reference control room, where alarm points are turned off by operator acknowledgement. An acknowledge-all action is therefore a convenience wrapper over the same per-alarm reset path used by [Reset](Alarms/Reset), useful after a transient has tripped dozens of alarms and the operator wants a clean slate to watch for anything new.

Source: /chapter_8/page_145.txt:66-67

Related controls: [Reset](Alarms/Reset)
Related sensors: [Active](Alarms/Active)

Source: specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:66-67
