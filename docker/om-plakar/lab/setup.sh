#!/bin/sh
# Prepare the lab: the store passphrase secret, the store section, the
# test/svc/app1 service and its witness files, with their checksums.
#
# Run in the container once the daemon is up:
#   docker exec om-plakar sh /lab/setup.sh
set -e

om backup/sec/lab create
om backup/sec/lab key add passphrase --value "$(head -c 32 /dev/urandom | base64)"

om cluster config update --set "store#lab.type=plakar" \
	--set "store#lab.store=/srv/kloset/lab" \
	--set "store#lab.passphrase=from backup/sec/lab key passphrase" \
	--set "store#lab.keep=7d 4w 6m"

mkdir -p /srv/app1
for i in 1 2 3 4 5 6 7 8 9 10; do
	head -c 1048576 /dev/urandom > /srv/app1/witness.$i
done
(cd /srv/app1 && sha256sum witness.* > /lab/app1.sha256)

om test/svc/app1 create --config /lab/app1.conf
om test/svc/app1 instance provision
