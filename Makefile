image:
	@printf "Enter HTTP proxy address for use when downloading code "\
	"from GitHub (e.g. http://username:password@example.com:port): " && \
	read PROXYADDR && (test -f Dockerfile.orig && true || cp Dockerfile Dockerfile.orig) && \
	(test -z "$$PROXYADDR" && true || sed "s#curl #curl -x '$$PROXYADDR' #" Dockerfile.orig > Dockerfile)
	docker build -t imgcrush . || true
	cp Dockerfile.orig Dockerfile && rm -f Dockerfile.orig
