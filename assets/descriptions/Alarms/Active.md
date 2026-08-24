JSON list of every currently active alarm with severity (**info** / **warning** / **trip**), human-readable description, and activation time. Rendered as structured rows in the annunciator panel rather than as a scalar readout, because the operator needs sequence-of-events information, not just a count.

Activation time is the forensic anchor: post-trip review starts from when each condition began, and the ordering of activation times reconstructs which failure triggered which protection response. Severity maps loosely to consequence - info items are informational, warnings threaten limits, trips correspond to conditions that actuate protection logic such as reactor trip or ESF actuation rather than merely annunciate.

Clicking a row acknowledges that single alarm through [Reset](Alarms/Reset).

Related controls: [Reset](Alarms/Reset)
Related sensors: [Alarms](Plant/Alarms)
