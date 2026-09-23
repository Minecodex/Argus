# Policy is compiled into a fixed trusted network sidecar. User containers
# cannot mount this filesystem or replace the immutable always-deny overlay.
FROM opensandbox/egress:v1.1.6@sha256:014d36e4a862c4a466c670b9df3f7516940b4b2aa440062438f09d1d8f9c9403
COPY deploy/workspace/deny.always /var/egress/rules/deny.always
