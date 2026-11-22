# 0005: Outside a modal, no action is bound to a printable key

A browse surface takes each printable key as text of its query, with no key to focus the query
first. An action on a printable key would never run there. The startup-error screen is the one
exception: it has no query and quits on `q`.

Rejected: actions on bare letters, because no surface can then take typed text as a filter.

Rejected: a Search tab with a `/` key that focuses its query, because to find an issue then takes
a tab switch, the focus key and Enter.
