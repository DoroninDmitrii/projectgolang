#!/usr/bin/env python3
"""Синхронизация версий из корневого Taskfile.yaml.

Читает vars: из корневого Taskfile.yaml и прокидывает их в:
  - Дочерние Taskfile.yaml/yml (*_VERSION переменные)
  - .env файлы (*_IMAGE переменные)
  - go.mod / go.work (версия Go)

Использование:
  sync-versions.py [подкоманда]

Подкоманды (examples):
  taskfiles   Обновить vars: в дочерних Taskfile
  env         Обновить .env файлы
  go          Обновить версию Go в go.mod и go.work
  deps        Обновить Go-зависимости (go get -u + go mod tidy + go work sync)
  all         Всё вместе (по умолчанию)

Подкоманды (homeworks):
  hw              Обновить всё в homeworks/ (taskfiles + env + go)
  hw:taskfiles    Обновить vars: в Taskfile домашек
  hw:env          Обновить .env файлы домашек
  hw:go           Обновить версию Go в go.mod и go.work домашек
"""

import argparse
import re
import subprocess
import sys
from pathlib import Path
from typing import Literal, NamedTuple

try:
    import yaml
except ImportError:
    print("❌ PyYAML не установлен. Установите: pip install pyyaml")
    sys.exit(1)

ROOT_DIR = Path(__file__).resolve().parent.parent
ROOT_TASKFILE = ROOT_DIR / "Taskfile.yaml"
HW_DIR = ROOT_DIR / "homeworks"

# Общие исключения для любого режима
_COMMON_EXCLUDES = {"node_modules", ".git"}
# examples: homeworks, solutions и archive исключены
EXCLUDE_DIRS = _COMMON_EXCLUDES | {"archive", "homeworks", "solutions"}
# homeworks: только общие исключения
HW_EXCLUDE_DIRS = _COMMON_EXCLUDES


class Result(NamedTuple):
    """Результат обновления."""
    updated: int
    total: int
    failed: int


EMPTY_RESULT = Result(0, 0, 0)


# ── Загрузка переменных ──────────────────────────────────────────────────────


def load_vars() -> dict[str, str]:
    """Загружает все vars из корневого Taskfile одним вызовом."""
    if not ROOT_TASKFILE.exists():
        print(f"❌ Не найден {ROOT_TASKFILE}")
        sys.exit(1)

    with open(ROOT_TASKFILE) as f:
        data = yaml.safe_load(f)
    return {k: str(v) for k, v in data.get("vars", {}).items()}


def split_vars(all_vars: dict[str, str]) -> tuple[dict[str, str], dict[str, str]]:
    """Разделяет переменные на VERSION и IMAGE группы."""
    versions = {k: v for k, v in all_vars.items() if k.endswith("_VERSION")}
    images = {k: v for k, v in all_vars.items() if k.endswith("_IMAGE")}
    return versions, images


def print_versions(versions: dict[str, str], images: dict[str, str]):
    """Выводит загруженные версии, группируя по типу."""
    print("📋 Загруженные версии:")
    for key, value in sorted(versions.items()):
        print(f"   {key}={value}")
    for key, value in sorted(images.items()):
        print(f"   {key}={value}")
    print()


# ── Поиск файлов ─────────────────────────────────────────────────────────────


def find_files(patterns: list[str], *,
               base_dir: Path | None = None,
               excludes: set[str] | None = None) -> list[Path]:
    """Ищет файлы по glob-паттернам, исключая служебные директории."""
    base = base_dir or ROOT_DIR
    excl = excludes if excludes is not None else EXCLUDE_DIRS
    results = []
    for pattern in patterns:
        for path in base.rglob(pattern):
            if not any(part in excl for part in path.parts):
                results.append(path)
    return sorted(set(results))


# ── Обновление файлов ─────────────────────────────────────────────────────────


Format = Literal["taskfile", "env"]


def update_file(path: Path, vars_dict: dict[str, str], fmt: Format) -> bool:
    """Обновляет все переменные в файле за одно чтение/запись."""
    text = path.read_text()
    new_text = text
    for key, value in vars_dict.items():
        if fmt == "taskfile":
            pattern = rf"^(\s+{re.escape(key)}:).*$"
            replacement = rf"\1 '{value}'"
        else:
            pattern = rf"^({re.escape(key)})=.*$"
            replacement = rf"\1={value}"
        new_text = re.sub(pattern, replacement, new_text, flags=re.MULTILINE)
    if new_text != text:
        path.write_text(new_text)
        return True
    return False


def update_files(label: str, files: list[Path], vars_dict: dict[str, str],
                 fmt: Format, skip: Path | None = None) -> Result:
    """Обновляет переменные в списке файлов, выводя прогресс."""
    print(f"═══ Обновление {label} ═══")
    print()

    target_files = [f for f in files if not skip or f.resolve() != skip.resolve()]
    if not target_files:
        print("  ⚠️  Нет файлов для обновления")
        print()
        return EMPTY_RESULT

    updated = 0
    for path in target_files:
        rel = path.relative_to(ROOT_DIR)
        if update_file(path, vars_dict, fmt):
            updated += 1
            print(f"  ✏️  {rel}")
        else:
            print(f"  ✅ {rel}")

    total = len(target_files)
    print()
    print(f"📊 {label}: проверено {total}, обновлено {updated}")
    print()
    return Result(updated, total, 0)


# ── Go-команды ───────────────────────────────────────────────────────────────


def _snapshot_go_files(work_dir: Path) -> dict[str, bytes]:
    """Снимает снапшот go.mod, go.sum, go.work в директории."""
    snapshot = {}
    for name in ("go.mod", "go.sum", "go.work"):
        p = work_dir / name
        if p.exists():
            snapshot[name] = p.read_bytes()
    return snapshot


def _run_go_commands(filename: str, icon: str,
                     commands: list[list[str]], *,
                     base_dir: Path | None = None,
                     excludes: set[str] | None = None) -> Result:
    """Запускает Go-команды для каждого найденного файла."""
    updated = checked = failed = 0
    for path in find_files([filename], base_dir=base_dir, excludes=excludes):
        work_dir = path.parent
        rel = work_dir.relative_to(ROOT_DIR)
        try:
            before = _snapshot_go_files(work_dir)
            for cmd in commands:
                subprocess.run(
                    cmd, cwd=work_dir, check=True,
                    capture_output=True, text=True,
                )
            after = _snapshot_go_files(work_dir)
            checked += 1
            if before != after:
                updated += 1
                print(f"  {icon} {filename}: {rel}")
            else:
                print(f"  ✅ {filename}: {rel}")
        except subprocess.CalledProcessError as e:
            failed += 1
            print(f"  ❌ {filename}: {rel}")
            print(f"     {e.stderr.strip()}")
    return Result(updated, checked, failed)


def _run_go_targets(label: str,
                    targets: list[tuple[str, str, list[list[str]]]], *,
                    base_dir: Path | None = None,
                    excludes: set[str] | None = None) -> Result:
    """Запускает Go-команды для списка целей (go.mod, go.work) и печатает итог."""
    print(f"═══ Обновление {label} ═══")
    print()

    total_updated = total_checked = total_failed = 0
    for filename, icon, commands in targets:
        r = _run_go_commands(filename, icon, commands,
                             base_dir=base_dir, excludes=excludes)
        total_updated += r.updated
        total_checked += r.total
        total_failed += r.failed

    total = total_checked + total_failed
    print()
    print(f"📊 {label}: проверено {total}, обновлено {total_updated}")
    print()
    return Result(total_updated, total, total_failed)


# ── Подкоманды ────────────────────────────────────────────────────────────────


def cmd_taskfiles(versions: dict[str, str]) -> Result:
    """Обновляет *_VERSION в дочерних Taskfile."""
    files = find_files(["Taskfile.yaml", "Taskfile.yml"])
    return update_files("Taskfile", files, versions, fmt="taskfile", skip=ROOT_TASKFILE)


def cmd_env(images: dict[str, str]) -> Result:
    """Обновляет *_IMAGE в .env файлах.

    Паттерн '*.env' может не ловить dotfiles в Python < 3.12,
    поэтому '.env' добавлен отдельно. Дедупликация через set() в find_files.
    """
    files = find_files(["*.env", ".env"])
    return update_files(".env", files, images, fmt="env")


def cmd_go(go_version: str) -> Result:
    """Обновляет версию Go в go.mod и go.work."""
    if not go_version:
        print("❌ GO_VERSION не задана в корневом Taskfile.yaml")
        sys.exit(1)

    return _run_go_targets(f"Go до {go_version}", [
        ("go.mod", "📦", [
            ["go", "mod", "edit", f"-go={go_version}"],
            ["go", "mod", "tidy"],
        ]),
        ("go.work", "🔧", [
            ["go", "work", "edit", f"-go={go_version}"],
            ["go", "work", "sync"],
        ]),
    ])


def cmd_deps() -> Result:
    """Обновляет Go-зависимости (go get -u + go mod tidy + go work sync)."""
    return _run_go_targets("Go-зависимостей", [
        ("go.mod", "📦", [
            ["go", "get", "-u", "./..."],
            ["go", "mod", "tidy"],
        ]),
        ("go.work", "🔧", [
            ["go", "work", "sync"],
        ]),
    ])


# ── Подкоманды: homeworks ────────────────────────────────────────────────────


def cmd_hw_taskfiles(versions: dict[str, str]) -> Result:
    """Обновляет *_VERSION в Taskfile домашек."""
    files = find_files(["Taskfile.yaml", "Taskfile.yml"],
                       base_dir=HW_DIR, excludes=HW_EXCLUDE_DIRS)
    return update_files("Homework Taskfile", files, versions, fmt="taskfile")


def cmd_hw_env(images: dict[str, str]) -> Result:
    """Обновляет *_IMAGE в .env файлах домашек."""
    files = find_files(["*.env", ".env"],
                       base_dir=HW_DIR, excludes=HW_EXCLUDE_DIRS)
    return update_files("Homework .env", files, images, fmt="env")


def cmd_hw_go(go_version: str) -> Result:
    """Обновляет версию Go в go.mod и go.work домашек.

    Без go mod tidy / go work sync — бойлерплейты неполные,
    студенты дописывают код сами.
    """
    if not go_version:
        print("❌ GO_VERSION не задана в корневом Taskfile.yaml")
        sys.exit(1)

    return _run_go_targets(f"Homework Go до {go_version}", [
        ("go.mod", "📦", [
            ["go", "mod", "edit", f"-go={go_version}"],
        ]),
        ("go.work", "🔧", [
            ["go", "work", "edit", f"-go={go_version}"],
        ]),
    ], base_dir=HW_DIR, excludes=HW_EXCLUDE_DIRS)


# ── Точка входа ───────────────────────────────────────────────────────────────


def main():
    parser = argparse.ArgumentParser(
        description="Синхронизация версий из корневого Taskfile.yaml",
    )
    all_commands = [
        "taskfiles", "env", "go", "deps", "all",
        "hw", "hw:taskfiles", "hw:env", "hw:go",
    ]
    parser.add_argument(
        "command",
        nargs="?",
        default="all",
        choices=all_commands,
        help="Подкоманда (по умолчанию: all)",
    )
    args = parser.parse_args()

    is_hw = args.command.startswith("hw")
    scope = "homeworks/" if is_hw else "examples/"
    print(f"🔄 Синхронизация версий из корневого Taskfile.yaml ({scope})...")
    print()

    all_vars = load_vars()
    versions, images = split_vars(all_vars)

    # Для deps версии не нужны — пропускаем вывод
    if args.command != "deps":
        print_versions(versions, images)

    summary: list[str] = []
    total_failed = 0

    # ── examples + solutions ─────────────────────────────────────────────────
    if args.command in ("taskfiles", "all"):
        r = cmd_taskfiles(versions)
        summary.append(f"  Taskfile: проверено {r.total}, обновлено {r.updated}")
        total_failed += r.failed

    if args.command in ("env", "all"):
        r = cmd_env(images)
        summary.append(f"  .env:     проверено {r.total}, обновлено {r.updated}")
        total_failed += r.failed

    if args.command in ("go", "all"):
        r = cmd_go(all_vars.get("GO_VERSION", ""))
        summary.append(f"  Go:       проверено {r.total}, обновлено {r.updated}")
        total_failed += r.failed

    if args.command in ("deps", "all"):
        r = cmd_deps()
        summary.append(f"  Deps:     проверено {r.total}, обновлено {r.updated}")
        total_failed += r.failed

    # ── homeworks ────────────────────────────────────────────────────────────
    if args.command in ("hw:taskfiles", "hw"):
        r = cmd_hw_taskfiles(versions)
        summary.append(f"  HW Taskfile: проверено {r.total}, обновлено {r.updated}")
        total_failed += r.failed

    if args.command in ("hw:env", "hw"):
        r = cmd_hw_env(images)
        summary.append(f"  HW .env:     проверено {r.total}, обновлено {r.updated}")
        total_failed += r.failed

    if args.command in ("hw:go", "hw"):
        r = cmd_hw_go(all_vars.get("GO_VERSION", ""))
        summary.append(f"  HW Go:       проверено {r.total}, обновлено {r.updated}")
        total_failed += r.failed

    # Резюме (только если больше одной части)
    if len(summary) > 1:
        print("════════════════════════════════════════════════════════════")
        print("                        РЕЗЮМЕ")
        print("════════════════════════════════════════════════════════════")
        print()
        for line in summary:
            print(line)
        print()
        print("════════════════════════════════════════════════════════════")

    if total_failed > 0:
        sys.exit(1)


if __name__ == "__main__":
    main()
