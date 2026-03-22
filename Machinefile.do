name: do-dev

os: linux
provider: digitalocean
region: london

resources:
  cpu: 2
  memory: 4gb

setup:
  - install: git
  - install: curl
  - clone:
      repo: https://github.com/octocat/Hello-World
      dest: /root/hello-world

run:
  - cmd: uname -a
  - cmd: cat /etc/os-release

expose:
  - 8080

