### release-automation

## Build
go build -o release-automation
## Start
./release-automation \
git@github.com:your/target-repo.git \
git@github.com:your/config-repo.git \  <-- push json repo \
develop \ <-- target repo branch \
1.0.0 \  <-- version \
label
