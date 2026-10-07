#!/usr/bin/env bash
# The integration-test mail server: Dovecot 2.4 (IMAP, LMTP) and Postfix
# (SMTP, submission) in the incus container frostmail-mailtest. Long-running
# services live in incus because idle nsl machines stop
# (docs/adr/0006-development-environment.md).
#
#   dev/incus/mailtest.sh up      create, configure, seed and snapshot `clean`
#   dev/incus/mailtest.sh reset [SNAPSHOT]  restore a snapshot (default `clean`)
#   dev/incus/mailtest.sh ip      print the container's IPv4 address
#   dev/incus/mailtest.sh seed USER N
#                                 import N mailgen messages (tools/mailgen,
#                                 seed 1) into USER's INBOX
#   dev/incus/mailtest.sh demo USER  deliver the demo messages (dev/incus/demo) to USER's INBOX
#   dev/incus/mailtest.sh snapshot NAME  replace snapshot NAME with the current state
#   dev/incus/mailtest.sh down    delete the container
#
# Accounts test1..test5@mailtest.test share the password in MAILTEST_PASSWORD.
# These are test-only values; nothing in the container delivers outside
# mailtest.test (default_transport and relay_transport are error:).
set -euo pipefail

NAME=frostmail-mailtest
IMAGE=images:debian/13
DOMAIN=mailtest.test
MAILTEST_PASSWORD=frostmail-test
HERE=$(cd "$(dirname "$0")" && pwd)

in_ct() { incus exec "$NAME" -- "$@"; }

ip4() {
	local ip
	for _ in $(seq 60); do
		ip=$(incus list "$NAME" -c 4 -f csv | awk '{print $1}')
		[[ -n "$ip" ]] && { echo "$ip"; return; }
		sleep 1
	done
	echo "no IPv4 address for $NAME" >&2
	return 1
}

up() {
	if incus info "$NAME" >/dev/null 2>&1; then
		echo "$NAME exists; use reset or down" >&2
		return 1
	fi
	incus launch "$IMAGE" "$NAME"
	ip4 >/dev/null
	in_ct sh -c 'until systemctl is-system-running --quiet 2>/dev/null || [ "$(systemctl is-system-running)" = degraded ]; do sleep 1; done'

	in_ct sh -c "echo 'postfix postfix/main_mailer_type select No configuration' | debconf-set-selections"
	in_ct env DEBIAN_FRONTEND=noninteractive apt-get update -q
	in_ct env DEBIAN_FRONTEND=noninteractive apt-get install -y -q --no-install-recommends \
		dovecot-imapd dovecot-lmtpd postfix ssl-cert ca-certificates

	in_ct groupadd -g 5000 vmail
	in_ct useradd -u 5000 -g vmail -d /var/vmail -m -s /usr/sbin/nologin vmail

	incus file push "$HERE/dovecot.conf" "$NAME/etc/dovecot/dovecot.conf"
	local users=""
	for i in 1 2 3 4 5; do
		users+="test$i@$DOMAIN:{PLAIN}$MAILTEST_PASSWORD::::::"$'\n'
	done
	printf '%s' "$users" | incus file push - "$NAME/etc/dovecot/users" --mode 0640
	in_ct chown root:dovecot /etc/dovecot/users
	in_ct doveconf -n >/dev/null

	# "No configuration" installs no main.cf; postconf -e fills an empty one.
	in_ct touch /etc/postfix/main.cf
	in_ct postconf -e \
		"compatibility_level = 3.10" \
		"myhostname = $DOMAIN" \
		"mydestination = localhost" \
		"inet_interfaces = all" \
		"inet_protocols = ipv4" \
		"mynetworks = 127.0.0.0/8" \
		"virtual_mailbox_domains = $DOMAIN" \
		"virtual_transport = lmtp:unix:private/dovecot-lmtp" \
		"default_transport = error:frostmail-mailtest delivers only to $DOMAIN" \
		"relay_transport = error:frostmail-mailtest delivers only to $DOMAIN" \
		"smtpd_tls_cert_file = /etc/ssl/certs/ssl-cert-snakeoil.pem" \
		"smtpd_tls_key_file = /etc/ssl/private/ssl-cert-snakeoil.key" \
		"smtpd_tls_security_level = may" \
		"smtpd_sasl_type = dovecot" \
		"smtpd_sasl_path = private/auth"
	in_ct postconf -M \
		"submission/inet=submission inet n - y - - smtpd -o syslog_name=postfix/submission -o smtpd_tls_security_level=encrypt -o smtpd_sasl_auth_enable=yes -o smtpd_client_restrictions=permit_sasl_authenticated,reject -o smtpd_recipient_restrictions=permit_sasl_authenticated,reject"

	# Postfix first: Dovecot binds its LMTP and auth sockets in Postfix's spool.
	in_ct systemctl restart postfix
	in_ct systemctl restart dovecot

	for f in "$HERE"/seed/*.eml; do
		in_ct doveadm save -u "test1@$DOMAIN" -m INBOX <"$f"
	done

	incus snapshot create "$NAME" clean
	echo "$NAME ready at $(ip4): IMAP 143/993, SMTP 25, submission 587"
}

reset() {
	incus snapshot restore "$NAME" "${1:-clean}"
	incus start "$NAME" 2>/dev/null || true
	ip4 >/dev/null
}

demo() {
	local user=${1:?usage: mailtest.sh demo USER}
	local f
	for f in "$HERE"/demo/*.eml; do
		incus exec "$NAME" -- doveadm save -u "$user@$DOMAIN" -m INBOX < "$f"
	done
	echo "delivered $(ls "$HERE"/demo/*.eml | wc -l) demo messages to $user@$DOMAIN"
}

seed() {
	local user=${1:?usage: mailtest.sh seed USER N} n=${2:?usage: mailtest.sh seed USER N}
	local tmp
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' RETURN
	(cd "$HERE/../.." && go run ./tools/mailgen -n "$n" -seed 1 -out "$tmp/Maildir")
	in_ct rm -rf /tmp/gen
	in_ct mkdir -p /tmp/gen
	tar -C "$tmp" -cf - Maildir | incus exec "$NAME" -- tar -C /tmp/gen -xf -
	# doveadm opens the source as a mailbox and writes its uidlist there.
	in_ct chown -R vmail:vmail /tmp/gen
	in_ct doveadm import -u "$user@$DOMAIN" maildir:/tmp/gen/Maildir "" all
	in_ct rm -rf /tmp/gen
	# doveadm import logs per-mailbox failures but still exits 0.
	local got
	got=$(in_ct doveadm -f flow mailbox status -u "$user@$DOMAIN" messages INBOX)
	got=${got##*messages=}
	if [[ "$got" != "$n" ]]; then
		echo "seed: $user@$DOMAIN INBOX has $got messages, want $n" >&2
		return 1
	fi
	echo "imported $n messages into $user@$DOMAIN"
}

snapshot() {
	local snap=${1:?usage: mailtest.sh snapshot NAME}
	incus snapshot delete "$NAME" "$snap" 2>/dev/null || true
	incus snapshot create "$NAME" "$snap"
}

case "${1:-}" in
up) up ;;
seed)
	shift
	seed "$@"
	;;
demo)
	shift
	demo "$@"
	;;
snapshot) snapshot "${2:-}" ;;
reset) reset "${2:-}" ;;
ip) ip4 ;;
down) incus delete --force "$NAME" ;;
*)
	echo "usage: $0 up|reset [SNAPSHOT]|ip|seed USER N|snapshot NAME|down" >&2
	exit 2
	;;
esac
