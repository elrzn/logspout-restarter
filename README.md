# logspout-restarter

Restart logspout if a log rotation is detected.

https://github.com/gliderlabs/logspout/issues/309

    docker run \
      --name logspout-restarter \
      -v /var/run/docker.sock:/var/run/docker.sock \
      -v /var/lib/docker/containers:/var/lib/docker/containers:ro \
      ${registry}/logspout-restarter -d

