---
trigger: always_on
---

проект работает в докере на десктопной версии windows
тесты вот так - docker run --rm -v ${PWD}:/app -w /app golang:1.25.3 go test и тд