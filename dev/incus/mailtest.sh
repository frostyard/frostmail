#!/usr/bin/env bash
# The integration-test mail server: Dovecot 2.4 (IMAP, LMTP) and Postfix
# (SMTP, submission) in the incus container frostmail-mailtest. Long-running
# services live in incus because idle nsl machines stop
# (docs/adr/0006-development-environment.md).
#
#   dev/incus/mailtest.sh up      create, configure, seed and snapshot `clean`
#   dev/incus/mailtest.sh reset   restore the `clean` snapshot
#   dev/incus/mailtest.sh ip      print the container's IPv4 address
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
	incus snapshot restore "$NAME" clean
	incus start "$NAME" 2>/dev/null || true
	ip4 >/dev/null
}

case "${1:-}" in
up) up ;;
reset) reset ;;
ip) ip4 ;;
down) incus delete --force "$NAME" ;;
*)
	echo "usage: $0 up|reset|ip|down" >&2
	exit 2
	;;
esac
