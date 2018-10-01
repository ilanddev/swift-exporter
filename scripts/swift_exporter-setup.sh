#!/bin/bash
##################################################
#  This script installs|removes Swift Exporter   #
##################################################

### CREATE ss-support user, set proper permissions and run as service as ss-support user.

basedir="/opt/ss/support"
tarball="swift_exporter_latest.tar.gz"

if [ $# -lt 1 ]
then
  echo "Usage: $0 install | remove"
  exit 1
fi

if [ $1 == "install" ]
then 
### Create directories where swift_exporter elements live

echo "Creating directories for binary"
sleep 1
   mkdir -p $basedir/downloads
   mkdir -p $basedir/swift_exporter/bin
   mkdir -p $basedir/swift_exporter/etc

echo "Figuring out if the OS is managaed by systemd or initd"
sleep 1
   systemd_or_initd=`cat /proc/1/comm`

if [ ! -f $basedir/downloads/$tarball ]; then
    curl https://cloud.swiftstack.com/v1/AUTH_apps/swift_exporter/swift_exporter_latest.tar.gz --output $basedir/downloads/$tarball
fi

# Do a conditional for version checking. if cloud_version > local_version ... wget/curl?? else exit;

### Extract files contained in tarball to proper locations

echo -n "Extracting files to directories..."
sleep 1
   tar -zxf $basedir/downloads/$tarball -C $basedir/swift_exporter/bin/ swift_exporter
   tar -zxf $basedir/downloads/$tarball -C $basedir/swift_exporter/etc/ swift_exporter_config.yaml
   tar -zxf $basedir/downloads/$tarball -C /etc/logrotate.d/ python-ssnode-with-su

  if [ $systemd_or_initd == "systemd" ]
  then
    echo "This OS is managed by systemd"
    tar -zxf $basedir/downloads/$tarball -C /usr/lib/systemd/system/ swift_exporter.service
  fi

  if [ $systemd_or_initd == "init" ]
  then
     echo "This OS is managed by init"
     tar -zxf $basedir/downloads/$tarball -C /etc/init/ swift_exporter.conf
     ln -s /etc/init/swift_exporter.conf /etc/init.d/swift_exporter
  fi

### Set execute permission on Swift Exporter binary and create symlink 

   chmod +x $basedir/swift_exporter/bin/swift_exporter
   ln -s $basedir/swift_exporter/bin/swift_exporter /usr/local/bin/swift_exporter

### Create Firewall Rule to allow port 53167

echo "Creating firewall rule. Opening TCP Port 53167"
sleep 1
   firewall-cmd --zone=public --add-port=53167/tcp ### CONSIDER CHANGING ZONE...TBD
### ADD CONDITIONAL FOR UBUNTU - UFW ###

### Enable and Start the Swift Exporter service

if [ $systemd_or_initd == "systemd" ]
then
  echo "Enabling Swift Exporter service..."
     systemctl enable swift_exporter.service

  echo "Starting Swift Exporter Service..."
     systemctl start swift_exporter.service

  echo -n "Swift Exporter Status is " && systemctl status swift_exporter.service  | grep Active | awk '{print $2}' | tr [a-z] [A-Z]
fi

if [ $systemd_or_initd == "init" ]
then
  echo "Enabling Swift Exporter service..."
     chkconfig swift_exporter on
  echo "Starting Swift Exporter Service..."
     service swift_exporter start
  echo -n "Swift Exporter Status is " && service swift_exporter status  | grep Active | awk '{print $2}' | tr [a-z] [A-Z]
fi

fi

# The below section is for removal of Swift Exporter

if [ $1 == "remove" ]
then

echo "Figuring out if the OS is managaed by systemd or initd"
sleep 1
   systemd_or_initd=`cat /proc/1/comm`

### Remove directories where swift_exporter elements live

   rm -rf $basedir/swift_exporter
   rm -f /usr/local/bin/swift_exporter

if [ $systemd_or_initd == "systemd" ]
then
  ### Remove Firewall Rule to allow port 53167
     firewall-cmd --zone=public --remove-port=53167/tcp
  ### SAME AS ABOVE - IE UBUNTU

  ### Disable and stop the Swift Exporter service
  echo "Stopping Swift Exporter Service..."
     systemctl stop swift_exporter.service

  echo "Disabling Swift Exporter service..."
     systemctl disable swift_exporter.service
     systemctl daemon-reload
     systemctl reset-failed

     rm /usr/lib/systemd/system/swift_exporter.service
     rm /opt/ss/support/downloads/swift_exporter*

     echo "Swift Exporter has been uninstalled..."
fi

if  [ $systemd_or_initd == "init" ]
then
  echo "Stopping Swift Exporter Service..."
     service swift_exporter stop

  echo "Disabling Swift Exporter service..."
     chkconfig swift_exporter off

     rm /etc/init.d/swift_exporter
     rm /etc/init/swift_exporter.conf
     rm /opt/ss/support/downloads/swift_exporter*

     echo "Swift Exporter has been uninstalled..."
fi

fi

