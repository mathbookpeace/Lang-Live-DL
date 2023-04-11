
apt-get update

apt-get install -y ca-certificates
update-ca-certificates

apt-get -y install golang
apt-get -y install ffmpeg

go run lang_live_dl.go