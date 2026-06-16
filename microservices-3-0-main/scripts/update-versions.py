#!/usr/bin/env python3
"""Обновление версий Go-инструментов и Docker-образов в корневом Taskfile.yaml.

Получает последние версии из GitHub (через gh CLI) и Docker Hub,
обновляет переменные *_VERSION и *_IMAGE в корневом Taskfile.yaml.

Docker-образы обновляются до последней стабильной версии.

Использование:
  update-versions.py              # Обновить всё (инструменты + образы)
  update-versions.py tools        # Только Go-инструменты
  update-versions.py images       # Только Docker-образы
  update-versions.py --dry-run    # Только показать доступные обновления
"""

import json
import re
import subprocess
import sys
import urllib.request
from pathlib import Path

ROOT_DIR = Path(__file__).resolve().parent.parent
ROOT_TASKFILE = ROOT_DIR / "Taskfile.yaml"

# ── Go-инструменты ───────────────────────────────────────────────────────────
# repo:       owner/repo на GitHub
# tag_prefix: префикс тега для фильтрации (монорепо)
# strip_v:    True если в Taskfile значение хранится БЕЗ префикса "v"

TOOLS = [
    {"var": "GOLANGCI_LINT_VERSION", "repo": "golangci/golangci-lint"},
    {"var": "GCI_VERSION",           "repo": "daixiang0/gci"},
    {"var": "GOFUMPT_VERSION",       "repo": "mvdan/gofumpt"},
    {"var": "BUF_VERSION",           "repo": "bufbuild/buf", "strip_v": True},
    {"var": "PROTOC_GEN_GO_VERSION", "repo": "protocolbuffers/protobuf-go"},
    {
        "var": "PROTOC_GEN_GO_GRPC_VERSION",
        "repo": "grpc/grpc-go",
        "tag_prefix": "cmd/protoc-gen-go-grpc/",
    },
    {"var": "OGEN_VERSION",          "repo": "ogen-go/ogen"},
    {"var": "MOCKERY_VERSION",       "repo": "vektra/mockery"},
    {"var": "GOOSE_VERSION",         "repo": "pressly/goose"},
    {"var": "YQ_VERSION",            "repo": "mikefarah/yq"},
    {"var": "ENVSUBST_VERSION",      "repo": "a8m/envsubst"},
]

# ── Docker-образы ────────────────────────────────────────────────────────────
# Два типа источников:
#   dockerhub: запрос Docker Hub Tags API, фильтрация по tag_re
#   github:    версия из GitHub releases, сборка полного image:tag
#
# tag_re:  regex для фильтрации тегов Docker Hub (берётся самый свежий стабильный)
# github:  owner/repo для GitHub releases
# image:   полное имя образа (для GitHub-backed)
# strip_v: убрать "v" из GitHub тега при формировании Docker-тега

IMAGES = [
    # ── Docker Hub: official images ──
    {
        "var": "POSTGRES_IMAGE",
        "dockerhub": "library/postgres",
        "tag_re": r"^\d+\.\d+-alpine\d+\.\d+$",
    },
    {
        "var": "MONGO_IMAGE",
        "dockerhub": "library/mongo",
        "tag_re": r"^\d+\.\d+$",
    },
    {
        "var": "REDIS_IMAGE",
        "dockerhub": "library/redis",
        "tag_re": r"^\d+\.\d+\.\d+-alpine\d+\.\d+$",
    },
    {
        "var": "NGINX_IMAGE",
        "dockerhub": "library/nginx",
        "tag_re": r"^\d+\.\d+\.\d+-alpine\d+\.\d+$",
    },
    # ── Docker Hub: vendor images ──
    {
        "var": "KAFKA_IMAGE",
        "dockerhub": "confluentinc/cp-kafka",
        "tag_re": r"^\d+\.\d+\.\d+$",
    },
    {
        "var": "KAFKA_LOCAL_IMAGE",
        "dockerhub": "confluentinc/confluent-local",
        "tag_re": r"^\d+\.\d+\.\d+$",
    },
    # ── GitHub-backed images ──
    {
        "var": "KAFKA_UI_IMAGE",
        "github": "kafbat/kafka-ui",
        "image": "ghcr.io/kafbat/kafka-ui",
    },
    {
        "var": "ELASTICSEARCH_IMAGE",
        "github": "elastic/elasticsearch",
        "image": "docker.elastic.co/elasticsearch/elasticsearch",
        "strip_v": True,
    },
    {
        "var": "KIBANA_IMAGE",
        "github": "elastic/kibana",
        "image": "docker.elastic.co/kibana/kibana",
        "strip_v": True,
    },
    {
        "var": "OTEL_COLLECTOR_IMAGE",
        "github": "open-telemetry/opentelemetry-collector-contrib",
        "image": "otel/opentelemetry-collector-contrib",
        "strip_v": True,
    },
    {
        "var": "PROMETHEUS_IMAGE",
        "github": "prometheus/prometheus",
        "image": "prom/prometheus",
    },
    {
        "var": "GRAFANA_IMAGE",
        "github": "grafana/grafana",
        "image": "grafana/grafana",
        "strip_v": True,
    },
    {
        "var": "JAEGER_IMAGE",
        "github": "jaegertracing/jaeger",
        "image": "jaegertracing/jaeger",
        "strip_v": True,
    },
    {
        "var": "ENVOY_IMAGE",
        "github": "envoyproxy/envoy",
        "image": "envoyproxy/envoy",
    },
]


# ── Утилиты ──────────────────────────────────────────────────────────────────


def parse_semver(version: str) -> tuple[int, ...]:
    """Парсит версию в кортеж чисел для сортировки.

    Извлекает ВСЕ числа из строки, что позволяет корректно сортировать
    теги вида '17.7-alpine3.23' → (17, 7, 3, 23).
    """
    return tuple(int(x) for x in re.findall(r"\d+", version))


def is_stable(version: str) -> bool:
    """Проверяет, что версия стабильная (без rc, alpha, beta, dev)."""
    return not re.search(r"(alpha|beta|rc|dev|pre|snapshot)", version, re.IGNORECASE)


def extract_major(tag: str) -> str:
    """Извлекает мажорную версию из тега: '17.7-alpine3.23' → '17'."""
    m = re.match(r"v?(\d+)", tag)
    return m.group(1) if m else ""


# ── GitHub API ───────────────────────────────────────────────────────────────


def gh_api(endpoint: str) -> dict | list:
    """Вызывает GitHub API через gh CLI."""
    result = subprocess.run(
        ["gh", "api", endpoint],
        capture_output=True, text=True, check=True,
    )
    return json.loads(result.stdout)


def get_latest_release(repo: str) -> str | None:
    """Получает последний стабильный релиз через GitHub API."""
    try:
        release = gh_api(f"repos/{repo}/releases/latest")
        return release["tag_name"]
    except subprocess.CalledProcessError:
        return None



def get_latest_tag(repo: str, prefix: str) -> str | None:
    """Получает последний стабильный тег с заданным префиксом.

    Использует git/refs/tags API с пагинацией — работает даже для
    монорепо с тысячами тегов (напр. grpc/grpc-go).
    """
    try:
        result = subprocess.run(
            ["gh", "api", f"repos/{repo}/git/refs/tags/{prefix}",
             "--paginate", "--jq", ".[].ref"],
            capture_output=True, text=True, check=True,
        )
    except subprocess.CalledProcessError:
        return None

    full_prefix = f"refs/tags/{prefix}"
    matching = []
    for line in result.stdout.strip().splitlines():
        if line.startswith(full_prefix):
            version = line[len(full_prefix):]
            if is_stable(version):
                matching.append(version)

    if not matching:
        return None

    matching.sort(key=parse_semver, reverse=True)
    return matching[0]


# ── Docker Hub API ───────────────────────────────────────────────────────────


def dockerhub_tags(repo: str, max_pages: int = 5) -> list[str]:
    """Получает список тегов из Docker Hub API."""
    url = f"https://hub.docker.com/v2/repositories/{repo}/tags?page_size=100"
    tags = []

    for _ in range(max_pages):
        req = urllib.request.Request(url)
        try:
            with urllib.request.urlopen(req, timeout=15) as resp:
                data = json.loads(resp.read())
        except (urllib.error.URLError, TimeoutError):
            break

        for result in data.get("results", []):
            name = result.get("name", "")
            if name:
                tags.append(name)

        url = data.get("next")
        if not url:
            break

    return tags


# ── Получение версий ─────────────────────────────────────────────────────────


def get_latest_tool_version(tool: dict) -> str | None:
    """Получает последнюю версию Go-инструмента из GitHub."""
    repo = tool["repo"]
    tag_prefix = tool.get("tag_prefix", "")
    strip_v = tool.get("strip_v", False)

    if tag_prefix:
        version = get_latest_tag(repo, tag_prefix)
    else:
        version = get_latest_release(repo)

    if version is None:
        return None

    if strip_v and version.startswith("v"):
        version = version[1:]

    return version


def get_latest_image(img: dict, current_value: str) -> str | None:
    """Получает последнюю стабильную версию Docker-образа.

    Для Docker Hub: фильтрует теги по паттерну, берёт самый свежий.
    Для GitHub-backed: берёт последний стабильный релиз.
    """
    image_name, _current_tag = current_value.rsplit(":", 1)

    if "dockerhub" in img:
        tags = dockerhub_tags(img["dockerhub"])
        matching = [t for t in tags if re.match(img["tag_re"], t) and is_stable(t)]
        if not matching:
            return None
        matching.sort(key=parse_semver, reverse=True)
        return f"{image_name}:{matching[0]}"

    if "github" in img:
        version = get_latest_release(img["github"])
        if version is None:
            return None
        tag = version
        if img.get("strip_v") and tag.startswith("v"):
            tag = tag[1:]
        return f"{img['image']}:{tag}"

    return None


# ── Обновление Taskfile ──────────────────────────────────────────────────────


def check_updates(items: list[dict], text: str, get_latest_fn) -> tuple[list, list]:
    """Проверяет обновления для списка элементов (tools или images).

    Возвращает (updates, errors).
    """
    updates = []
    errors = []

    for item in items:
        var_name = item["var"]
        match = re.search(
            rf"^\s+{re.escape(var_name)}:\s+'([^']+)'", text, re.MULTILINE,
        )
        if not match:
            errors.append(f"  ❌ {var_name}: не найдена в Taskfile")
            continue

        current = match.group(1)

        try:
            latest = get_latest_fn(item, current)
        except Exception as e:
            errors.append(f"  ❌ {var_name}: ошибка — {e}")
            continue

        if latest is None:
            errors.append(f"  ❌ {var_name}: не удалось получить версию")
            continue

        if current == latest:
            print(f"  ✅ {var_name}: {current}")
        else:
            print(f"  ⬆️  {var_name}: {current} → {latest}")
            updates.append((var_name, current, latest))

    return updates, errors


def apply_updates(text: str, updates: list[tuple[str, str, str]]) -> str:
    """Применяет обновления к тексту Taskfile."""
    for var_name, _current, latest in updates:
        pattern = rf"^(\s+{re.escape(var_name)}:)\s+'[^']+'$"
        replacement = rf"\1 '{latest}'"
        text = re.sub(pattern, replacement, text, flags=re.MULTILINE)
    return text


# ── Точка входа ──────────────────────────────────────────────────────────────


def main():
    dry_run = "--dry-run" in sys.argv
    args = [a for a in sys.argv[1:] if not a.startswith("-")]
    command = args[0] if args else "all"

    if command not in ("all", "tools", "images"):
        print(f"Использование: {sys.argv[0]} [all|tools|images] [--dry-run]")
        sys.exit(1)

    mode_label = " (dry-run)" if dry_run else ""
    print(f"🔍 Проверка обновлений{mode_label}...")
    print()

    text = ROOT_TASKFILE.read_text()
    all_updates = []
    all_errors = []

    # Go-инструменты
    if command in ("all", "tools"):
        print("── Go-инструменты ──")
        print()
        updates, errors = check_updates(
            TOOLS, text,
            lambda item, current: get_latest_tool_version(item),
        )
        all_updates.extend(updates)
        all_errors.extend(errors)
        print()

    # Docker-образы
    if command in ("all", "images"):
        print("── Docker-образы ──")
        print()
        updates, errors = check_updates(
            IMAGES, text,
            lambda item, current: get_latest_image(item, current),
        )
        all_updates.extend(updates)
        all_errors.extend(errors)
        print()

    for err in all_errors:
        print(err)
    if all_errors:
        print()

    if not all_updates:
        print("✅ Все версии актуальны!")
        return

    print(f"📋 Доступно обновлений: {len(all_updates)}")

    if dry_run:
        print("   (--dry-run, файл не изменён)")
        return

    print()
    text = apply_updates(text, all_updates)
    ROOT_TASKFILE.write_text(text)
    print(f"✏️  Обновлено {len(all_updates)} версий в Taskfile.yaml")
    print()
    print("💡 Не забудь запустить: task sync")


if __name__ == "__main__":
    main()
