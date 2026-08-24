Structured feed of active alarm conditions aggregated across all modeled subsystems; drives the dedicated annunciator panel above the tile grid.

Alarm-kind sensors are routed away from the scalar sensor table deliberately: their payload is a JSON array of structured rows (severity, description, activation time, acknowledgment state) that deserves its own rendering with severity badges and click-to-acknowledge behavior. Escalation runs info - warning - trip, where trip-class alarms correspond to conditions wired into protection logic rather than mere annunciation.

Every active alarm carries its activation time, which is what makes the panel usable as a crude sequence-of-events recorder during transients.

Related controls: [Reset](Alarms/Reset)
Related sensors: [Active](Alarms/Active)
