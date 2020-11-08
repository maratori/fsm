SRCS := $(wildcard *.puml)
PNGS := $(SRCS:%.puml=%.png)
SVGS := $(SRCS:%.puml=%.svg)

gen: docker-build $(PNGS) $(SVGS)
.PHONY: gen

%.png: %.puml
	cat $< | docker run --rm -i fsm:plantuml -tpng > $@

%.svg: %.puml
	cat $< | docker run --rm -i fsm:plantuml > $@

docker-build:
	docker build --tag fsm:plantuml - < Dockerfile
.PHONY: docker-build
