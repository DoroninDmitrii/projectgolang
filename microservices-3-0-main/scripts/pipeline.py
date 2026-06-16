#!/usr/bin/env python3
"""Pipeline-скрипт: clean, run-task, all (полный пайплайн).

Замена трёх bash-скриптов (clean.sh, run-task.sh, pipeline.sh)
единым Python-модулем с субкомандами.

Использование:
  pipeline.py clean     <weeks>
  pipeline.py run-task  <task> <weeks> [OPTIONS]
  pipeline.py all       <weeks> [--clean]
"""

import argparse
import os
import re
import shutil
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path

ROOT_DIR = Path(__file__).resolve().parent.parent

EXCLUDE_DIRS = {"boilerplate", "boilerplates", "archive"}

SEARCH_ROOTS = ["examples"]

# ── Метаданные задач ─────────────────────────────────────────────────────────

TASK_META: dict[str, tuple[str, str, str]] = {
    # task_name: (emoji, label_verb, title_noun)
    "build":      ("🔨", "Собираем",            "Сборка"),
    "format":     ("🧹", "Форматируем",        "Формат"),
    "lint":       ("🔍", "Линтим",             "Линтер"),
    "mocks:gen":  ("🎭", "Генерируем моки",    "Моки"),
    "proto:gen":  ("🔨", "Генерируем proto",   "Proto"),
    "ogen:gen":   ("🔨", "Генерируем OpenAPI",  "OpenAPI"),
    "test:api": ("🧪", "Тестируем",          "API-тесты"),
}


def get_task_meta(task_name: str) -> tuple[str, str, str]:
    """Возвращает (emoji, label, title) для задачи."""
    return TASK_META.get(task_name, ("📦", task_name, task_name))


# ── Работа с неделями ────────────────────────────────────────────────────────


@dataclass(frozen=True)
class WeekSpec:
    """Распарсенная спецификация недель."""

    patterns: list[str]
    label: str

    @classmethod
    def parse(cls, weeks_str: str) -> "WeekSpec":
        stripped = weeks_str.strip()
        if stripped == "*":
            return cls(patterns=["week_*"], label="все примеры")
        parts = stripped.split()
        patterns = [f"week_{w}" for w in parts]
        if len(parts) == 1:
            label = f"week_{parts[0]}"
        else:
            label = "week_{" + ",".join(parts) + "}"
        return cls(patterns=patterns, label=label)


# ── Поиск директорий ─────────────────────────────────────────────────────────


def _has_taskskip(path: Path) -> bool:
    """Проверяет, есть ли .taskskip в директории или её родителях до ROOT_DIR."""
    current = path
    while current != ROOT_DIR and current != current.parent:
        if (current / ".taskskip").exists():
            return True
        current = current.parent
    return False


def _is_excluded(path: Path) -> bool:
    """Проверяет, содержит ли путь исключённые директории."""
    return any(part in EXCLUDE_DIRS for part in path.parts)


def _search_roots() -> list[Path]:
    """Возвращает существующие корневые директории для поиска."""
    return [ROOT_DIR / name for name in SEARCH_ROOTS if (ROOT_DIR / name).exists()]


def find_dirs_by_file(week_patterns: list[str], filename: str) -> list[Path]:
    """Ищет директории, содержащие указанный файл, внутри week_* папок."""
    dirs: set[Path] = set()
    for search_root in _search_roots():
        for week_pat in week_patterns:
            # Ищем на любой глубине вложенности внутри week_* папок
            for match in search_root.glob(f"**/{week_pat}/**/{filename}"):
                if not _is_excluded(match):
                    dirs.add(match.parent)
    return sorted(dirs)


def find_taskfile_dirs(week_patterns: list[str]) -> list[Path]:
    """Ищет директории с Taskfile.yaml или Taskfile.yml."""
    yaml_dirs = find_dirs_by_file(week_patterns, "Taskfile.yaml")
    yml_dirs = find_dirs_by_file(week_patterns, "Taskfile.yml")
    return sorted(set(yaml_dirs) | set(yml_dirs))


def find_subdirs(week_patterns: list[str], dirname: str) -> list[Path]:
    """Ищет все поддиректории с указанным именем внутри week_* папок."""
    results: set[Path] = set()
    for search_root in _search_roots():
        for week_pat in week_patterns:
            for match in search_root.glob(f"**/{week_pat}/**/{dirname}"):
                if match.is_dir() and not _is_excluded(match):
                    results.add(match)
    return sorted(results)


def find_go_module_dirs(week_patterns: list[str]) -> list[Path]:
    """Ищет директории с go.mod (не внутри go.work), и директории с go.work."""
    go_work_dirs: set[Path] = set()
    go_mod_dirs: set[Path] = set()

    for search_root in _search_roots():
        for week_pat in week_patterns:
            # Сначала собираем go.work
            for match in search_root.glob(f"**/{week_pat}/**/go.work"):
                if not _is_excluded(match):
                    go_work_dirs.add(match.parent)

            # Потом go.mod
            for match in search_root.glob(f"**/{week_pat}/**/go.mod"):
                if not _is_excluded(match):
                    mod_dir = match.parent
                    # Пропускаем go.mod, если он внутри go.work директории
                    is_inside_workspace = any(
                        mod_dir == wd or mod_dir.is_relative_to(wd)
                        for wd in go_work_dirs
                    )
                    if not is_inside_workspace:
                        go_mod_dirs.add(mod_dir)

    return sorted(go_work_dirs | go_mod_dirs)


def _filter_taskskip(dirs: list[Path]) -> list[Path]:
    """Фильтрует директории с .taskskip, печатая пропуски."""
    result = []
    for d in dirs:
        if _has_taskskip(d):
            rel = d.relative_to(ROOT_DIR)
            print(f"  ⏭️  Пропускаем (.taskskip): ./{rel}")
            continue
        result.append(d)
    return result


# ── Отчёт о выполнении ──────────────────────────────────────────────────────


@dataclass
class StageReport:
    """Результат выполнения одного этапа пайплайна."""

    label: str = ""
    emoji: str = ""
    total: int = 0
    failed: int = 0
    skipped: int = 0
    failed_list: list[str] = field(default_factory=list)
    skipped_list: list[str] = field(default_factory=list)
    bin_deleted: int = 0
    mocks_deleted: int = 0

    @property
    def success(self) -> int:
        return self.total - self.failed


# ── Красивый вывод ───────────────────────────────────────────────────────────

SEPARATOR = "════════════════════════════════════════════════════════════"


def _print_header(title: str) -> None:
    print()
    print(SEPARATOR)
    print(f"  {title}")
    print(SEPARATOR)
    print()


def _print_standalone_report(report: StageReport) -> None:
    """Печатает человекочитаемый отчёт (режим standalone)."""
    _print_header("РЕЗЮМЕ")

    if report.success > 0:
        print(f"  ✅ Успешно: {report.success}")
        print()

    if report.failed > 0:
        print(f"  ❌ С ошибками ({report.failed}):")
        for item in report.failed_list:
            print(f"     • {item}")
        print()

    if report.total == 0:
        print("  📭 Не найдено проектов")
        print()

    print(SEPARATOR)


# ── Команда: build ───────────────────────────────────────────────────────────


def _go_work_build_patterns(work_dir: Path) -> list[str]:
    """Возвращает паттерны ./module/... для каждого use-модуля в go.work."""
    go_work = work_dir / "go.work"
    patterns: list[str] = []
    for line in go_work.read_text().splitlines():
        line = line.strip()
        # Строки вида "./submodule" внутри блока use (...)
        if line.startswith("./"):
            patterns.append(f"{line}/...")
    return patterns if patterns else ["./..."]


def _do_build(spec: WeekSpec) -> StageReport:
    """Собирает все Go-модули (go build ./...)."""
    emoji, verb, title = get_task_meta("build")
    print(f"{emoji} {verb} для {spec.label}...")
    print()

    dirs = find_go_module_dirs(spec.patterns)
    dirs = _filter_taskskip(dirs)

    report = StageReport(label=title, emoji=emoji)

    for d in dirs:
        rel = f"./{d.relative_to(ROOT_DIR)}"
        report.total += 1
        print(f"  📁 {verb}: {rel}")

        # Для workspace — собираем каждый use-модуль явно
        if (d / "go.work").exists():
            build_args = _go_work_build_patterns(d)
        else:
            build_args = ["./..."]

        result = subprocess.run(
            ["go", "build"] + build_args,
            cwd=str(d),
        )

        if result.returncode != 0:
            report.failed += 1
            report.failed_list.append(rel)

    return report


def cmd_build(weeks_str: str) -> int:
    """CLI-обёртка для build."""
    spec = WeekSpec.parse(weeks_str)
    report = _do_build(spec)
    _print_standalone_report(report)
    return 1 if report.failed > 0 else 0


def _print_clean_report(report: StageReport) -> None:
    """Печатает отчёт очистки (режим standalone)."""
    _print_header("РЕЗЮМЕ")
    if report.bin_deleted > 0:
        print(f"  🗑️  Удалено bin: {report.bin_deleted}")
    if report.mocks_deleted > 0:
        print(f"  🗑️  Удалено mocks: {report.mocks_deleted}")
    if report.bin_deleted == 0 and report.mocks_deleted == 0:
        print("  📭 Нечего удалять")
    print()
    print(SEPARATOR)


def _print_stage_summary(report: StageReport) -> None:
    """Печатает строку результата этапа в сводный отчёт."""
    if report.total == 0:
        print(f"  {report.emoji} {report.label}: нет проектов")
    elif report.failed == 0:
        print(f"  {report.emoji} {report.label}: ✅ все {report.success} успешно")
    else:
        print(f"  {report.emoji} {report.label}: ✅ {report.success}  ❌ {report.failed}")
        for item in report.failed_list:
            print(f"     • {item}")

    if report.skipped > 0:
        print(f"     Без тестов ({report.skipped}):")
        for item in report.skipped_list:
            print(f"     · {item}")


# ── Команда: clean ───────────────────────────────────────────────────────────


def _do_clean(spec: WeekSpec) -> StageReport:
    """Удаляет директории bin/ и mocks/ внутри week_* папок."""
    print(f"🗑️  Удаляем директории bin и mocks для {spec.label}...")
    print()

    report = StageReport(label="Очистка", emoji="🗑️")

    for dirname in ("bin", "mocks"):
        dirs = find_subdirs(spec.patterns, dirname)
        dirs = _filter_taskskip(dirs)
        for d in dirs:
            rel = d.relative_to(ROOT_DIR)
            print(f"  🗑️  Удаляем: ./{rel}")
            shutil.rmtree(d, ignore_errors=True)
            if dirname == "bin":
                report.bin_deleted += 1
            else:
                report.mocks_deleted += 1

    return report


def cmd_clean(weeks_str: str) -> int:
    """CLI-обёртка для clean."""
    spec = WeekSpec.parse(weeks_str)
    report = _do_clean(spec)
    _print_clean_report(report)
    return 0


# ── Команда: clean-gen ──────────────────────────────────────────────────────

# Директории со сгенерированным кодом (proto, OpenAPI, swagger)
GEN_DIRS = ("pkg/proto", "pkg/openapi", "static/generated")


def _do_clean_gen(spec: WeekSpec) -> StageReport:
    """Удаляет bin/ и сгенерированные файлы (pkg/proto, pkg/openapi, static/generated)."""
    print(f"🗑️  Удаляем bin и сгенерированный код для {spec.label}...")
    print()

    report = StageReport(label="Очистка (regen)", emoji="🗑️")

    # Удаляем bin/ (плагины protoc, ogen и пр.)
    bin_dirs = find_subdirs(spec.patterns, "bin")
    bin_dirs = _filter_taskskip(bin_dirs)
    for d in bin_dirs:
        rel = d.relative_to(ROOT_DIR)
        print(f"  🗑️  Удаляем: ./{rel}")
        shutil.rmtree(d, ignore_errors=True)
        report.bin_deleted += 1

    # Удаляем директории со сгенерированным кодом
    gen_deleted = 0
    for gen_dir in GEN_DIRS:
        dirs = find_subdirs(spec.patterns, gen_dir)
        dirs = _filter_taskskip(dirs)
        for d in dirs:
            rel = d.relative_to(ROOT_DIR)
            print(f"  🗑️  Удаляем: ./{rel}")
            shutil.rmtree(d, ignore_errors=True)
            gen_deleted += 1

    report.mocks_deleted = gen_deleted  # reuse field for summary

    return report


def _print_clean_gen_report(report: StageReport) -> None:
    """Печатает отчёт очистки сгенерированного кода."""
    _print_header("РЕЗЮМЕ")
    if report.bin_deleted > 0:
        print(f"  🗑️  Удалено bin: {report.bin_deleted}")
    if report.mocks_deleted > 0:
        print(f"  🗑️  Удалено сгенерированных директорий: {report.mocks_deleted}")
    if report.bin_deleted == 0 and report.mocks_deleted == 0:
        print("  📭 Нечего удалять")
    print()
    print(SEPARATOR)


def cmd_clean_gen(weeks_str: str) -> int:
    """CLI-обёртка для clean-gen."""
    spec = WeekSpec.parse(weeks_str)
    report = _do_clean_gen(spec)
    _print_clean_gen_report(report)
    return 0


# ── Команда: run-task ────────────────────────────────────────────────────────


def _task_exists_in_file(taskfile: Path, task_name: str) -> bool:
    """Проверяет наличие задачи в Taskfile (grep по файлу)."""
    if not taskfile.exists():
        return False
    pattern = re.compile(rf"^\s+{re.escape(task_name)}:", re.MULTILINE)
    return bool(pattern.search(taskfile.read_text()))


def _find_taskfile(directory: Path) -> Path | None:
    """Находит Taskfile.yaml или Taskfile.yml в директории."""
    for name in ("Taskfile.yaml", "Taskfile.yml"):
        path = directory / name
        if path.exists():
            return path
    return None


def _do_run_task(
    task_name: str,
    spec: WeekSpec,
    find_by: str | None = None,
    check_exists: bool = False,
) -> StageReport:
    """Запускает task-команду во всех найденных директориях."""
    emoji, verb, title = get_task_meta(task_name)

    print(f"{emoji} {verb} для {spec.label}...")
    print()

    # Поиск директорий
    if find_by:
        dirs = find_dirs_by_file(spec.patterns, find_by)
    else:
        dirs = find_taskfile_dirs(spec.patterns)

    dirs = _filter_taskskip(dirs)

    report = StageReport(label=title, emoji=emoji)

    for d in dirs:
        rel = f"./{d.relative_to(ROOT_DIR)}"

        # Проверяем наличие задачи в Taskfile
        if check_exists:
            taskfile = _find_taskfile(d)
            if not taskfile or not _task_exists_in_file(taskfile, task_name):
                report.skipped += 1
                report.skipped_list.append(rel)
                continue

        report.total += 1
        print(f"  📁 {verb}: {rel}")

        # Для линтера — изолированный кеш
        env = os.environ.copy()
        if task_name == "lint":
            env["GOLANGCI_LINT_CACHE"] = str(d.resolve() / ".golangci-cache")

        result = subprocess.run(
            ["task", "-d", str(d), task_name],
            env=env,
            cwd=ROOT_DIR,
        )

        if result.returncode != 0:
            report.failed += 1
            report.failed_list.append(rel)

    return report


def cmd_run_task(
    task_name: str,
    weeks_str: str,
    find_by: str | None = None,
    check_exists: bool = False,
) -> int:
    """CLI-обёртка для run-task."""
    spec = WeekSpec.parse(weeks_str)
    report = _do_run_task(task_name, spec, find_by=find_by, check_exists=check_exists)
    _print_standalone_report(report)
    return 1 if report.failed > 0 else 0


# ── Команда: all (pipeline) ─────────────────────────────────────────────────


PIPELINE_STAGES = [
    # (task_name, extra_args)
    ("build",      {}),
    ("mocks:gen",  {"find_by": ".mockery.yaml"}),
    ("format",     {}),
    ("lint",       {}),
    ("test:api", {"check_exists": True}),
]


def cmd_all(weeks_str: str, do_clean: bool = False) -> int:
    """Полный пайплайн: [clean] -> mocks:gen -> format -> lint -> test:proto."""
    spec = WeekSpec.parse(weeks_str)
    print(f"🚀 Запускаем полный пайплайн для {spec.label}...")
    print()

    clean_report = None
    if do_clean:
        clean_report = _do_clean(spec)

    # Запуск каждого этапа, собираем отчёты напрямую
    stage_reports = []
    for task_name, extra in PIPELINE_STAGES:
        if task_name == "build":
            report = _do_build(spec)
        else:
            report = _do_run_task(
                task_name=task_name,
                spec=spec,
                find_by=extra.get("find_by"),
                check_exists=extra.get("check_exists", False),
            )
        stage_reports.append(report)

    # Сводный отчёт
    has_errors = False
    _print_header(f"СВОДНЫЙ ОТЧЁТ — {spec.label}")

    if clean_report:
        if clean_report.bin_deleted > 0 or clean_report.mocks_deleted > 0:
            print(f"  🗑️  Очистка: удалено bin: {clean_report.bin_deleted}, "
                  f"mocks: {clean_report.mocks_deleted}")
        else:
            print("  🗑️  Очистка: нечего удалять")

    for report in stage_reports:
        _print_stage_summary(report)
        if report.failed > 0:
            has_errors = True

    print()
    if has_errors:
        print("  ❌ Некоторые этапы завершились с ошибками")
    else:
        print("  ✅ Все этапы завершены успешно!")
    print()
    print(SEPARATOR)

    return 1 if has_errors else 0


# ── CLI ──────────────────────────────────────────────────────────────────────


def main() -> None:
    # Отключаем буферизацию stdout, чтобы прогресс чередовался с выводом задач
    sys.stdout.reconfigure(line_buffering=True)

    parser = argparse.ArgumentParser(
        description="Pipeline: clean, run-task, all",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    sub = parser.add_subparsers(dest="command", required=True)

    # clean
    p_clean = sub.add_parser("clean", help="Удаление bin/ и mocks/")
    p_clean.add_argument("weeks", help="Номера недель (напр. '1 2 3' или '*')")

    # clean-gen
    p_clean_gen = sub.add_parser("clean-gen", help="Удаление bin/ и сгенерированного кода")
    p_clean_gen.add_argument("weeks", help="Номера недель (напр. '1 2 3' или '*')")

    # build
    p_build = sub.add_parser("build", help="Сборка Go-модулей (go build ./...)")
    p_build.add_argument("weeks", help="Номера недель (напр. '1 2 3' или '*')")

    # run-task
    p_run = sub.add_parser("run-task", help="Запуск task-команды по директориям")
    p_run.add_argument("task", help="Имя задачи (format, lint, mocks:gen, ...)")
    p_run.add_argument("weeks", help="Номера недель")
    p_run.add_argument("--find-by", help="Искать проекты по файлу вместо Taskfile")
    p_run.add_argument("--check-exists", action="store_true",
                       help="Пропускать, если задачи нет в Taskfile")

    # all
    p_all = sub.add_parser("all", help="Полный пайплайн")
    p_all.add_argument("weeks", help="Номера недель")
    p_all.add_argument("--clean", action="store_true",
                       help="Включить этап очистки перед пайплайном")

    args = parser.parse_args()

    if args.command == "clean":
        rc = cmd_clean(args.weeks)
    elif args.command == "clean-gen":
        rc = cmd_clean_gen(args.weeks)
    elif args.command == "build":
        rc = cmd_build(args.weeks)
    elif args.command == "run-task":
        rc = cmd_run_task(
            task_name=args.task,
            weeks_str=args.weeks,
            find_by=args.find_by,
            check_exists=args.check_exists,
        )
    elif args.command == "all":
        rc = cmd_all(args.weeks, do_clean=args.clean)
    else:
        parser.print_help()
        rc = 1

    sys.exit(rc)


if __name__ == "__main__":
    main()
