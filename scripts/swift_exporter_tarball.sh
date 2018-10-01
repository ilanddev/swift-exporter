#!/bin/bash

EXPORTER_VERSION=$(./swift_exporter_go/swift_exporter --version)
PKG_VERSION="swift_exporter_$EXPORTER_VERSION"

echo
printf "* Packaging version: $PKG_VERSION \n"

cp ./swift_exporter_go/swift_exporter                                  swift_exporter_go/pkg/files/
cp ./swift_exporter_go/swift_exporter_config.yaml                      swift_exporter_go/pkg/files/
cp ./swift_exporter_go/usr/lib/systemd/system/swift_exporter.service   swift_exporter_go/pkg/files/
cp ./swift_exporter_go/etc/logrotate.d/python-ssnode-with-su           swift_exporter_go/pkg/files/
cp ./swift_exporter_go/etc/init/swift_exporter.conf                    swift_exporter_go/pkg/files/

MD5SUM=$(cd swift_exporter_go && md5sum swift_exporter)

printf "* MD5SUM check: $MD5SUM \n"

echo "$MD5SUM" > swift_exporter_go/pkg/files/swift_exporter_md5sum.txt

cd ./swift_exporter_go/pkg/files
tar -cvzf $PKG_VERSION.tar.gz * > /dev/null

cd ../../..
mv ./swift_exporter_go/pkg/files/$PKG_VERSION.tar.gz      ./swift_exporter_go/pkg/tarballs/
cp ./swift_exporter_go/pkg/tarballs/$PKG_VERSION.tar.gz   ./swift_exporter_go/pkg/tarballs/swift_exporter_latest.tar.gz

printf "* Please upload the tarballs in 'swift_exporter_go/pkg/tarballs/' to: 

  https://cloud.swiftstack.com/v1/AUTH_support/prometheus/ \n\n"

printf "* Please aldo upload the MD5 sum in 'swift_exporter_go/pkg/files/' to:
  https://cloud.swiftstack.com/v1/AUTH_support/prometheus/ \n\n"
