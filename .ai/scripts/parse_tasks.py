#!/usr/bin/env python3
"""
parse_tasks.py - Parse tasks.md with dependency support (Task DAG)

Usage:
    python3 .ai/scripts/parse_tasks.py <tasks_file> [--json] [--next] [--parallel]

Options:
    --json      Output as JSON
    --next      Show next executable task(s)
    --parallel  Show all tasks that can run in parallel

Dependency syntax in tasks.md:
    - [ ] 1. First task
    - [ ] 2. Second task
      - _depends_on: 1_
    - [ ] 3. Third task
      - _depends_on: 1, 2_
"""

import re
import sys
import json
from pathlib import Path
from collections import defaultdict
from typing import Dict, List, Set, Tuple, Optional

from lib.errors import AWKError, ConfigError, handle_unexpected_error, print_error
from lib.logger import Logger, split_log_level

class Task:
    def __init__(self, id: str, title: str, completed: bool = False):
        self.id = id
        self.title = title
        self.completed = completed
        self.depends_on: List[str] = []
        self.subtasks: List['Task'] = []
    
    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "title": self.title,
            "completed": self.completed,
            "depends_on": self.depends_on,
            "subtasks": [s.to_dict() for s in self.subtasks]
        }

def parse_tasks(content: str) -> List[Task]:
    """Parse tasks.md content and extract tasks with dependencies."""
    tasks = []
    current_task: Optional[Task] = None
    
    lines = content.split('\n')
    
    # Pattern for main tasks: - [ ] 1. Title or - [x] 1. Title
    task_pattern = re.compile(r'^- \[([ x])\]\s*(\d+)\.\s*(.+)$')
    # Pattern for subtasks: - [ ] 1.1 Title
    subtask_pattern = re.compile(r'^\s+- \[([ x])\]\*?\s*(\d+\.\d+)\s*(.+)$')
    # Pattern for dependencies: - _depends_on: 1, 2_
    depends_pattern = re.compile(r'^\s+-?\s*_depends_on:\s*([^_]+)_')
    
    for line in lines:
        # Check for main task
        task_match = task_pattern.match(line)
        if task_match:
            completed = task_match.group(1) == 'x'
            task_id = task_match.group(2)
            title = task_match.group(3).strip()
            current_task = Task(task_id, title, completed)
            tasks.append(current_task)
            continue
        
        # Check for subtask
        subtask_match = subtask_pattern.match(line)
        if subtask_match and current_task:
            completed = subtask_match.group(1) == 'x'
            subtask_id = subtask_match.group(2)
            title = subtask_match.group(3).strip()
            subtask = Task(subtask_id, title, completed)
            current_task.subtasks.append(subtask)
            continue
        
        # Check for dependency declaration
        depends_match = depends_pattern.match(line)
        if depends_match and current_task:
            deps = depends_match.group(1).strip()
            dep_ids = [d.strip() for d in deps.split(',') if d.strip()]
            current_task.depends_on.extend(dep_ids)
    
    return tasks

def build_dependency_graph(tasks: List[Task]) -> Dict[str, Set[str]]:
    """Build adjacency list for dependency graph."""
    graph = defaultdict(set)
    for task in tasks:
        graph[task.id]  # Ensure all tasks are in graph
        for dep in task.depends_on:
            graph[task.id].add(dep)
    return graph

def topological_sort(tasks: List[Task]) -> List[Task]:
    """Return tasks in topological order (dependencies first)."""
    task_map = {t.id: t for t in tasks}
    graph = build_dependency_graph(tasks)
    
    # Kahn's algorithm
    in_degree = defaultdict(int)
    for task_id in graph:
        in_degree[task_id]  # Initialize
    for task_id, deps in graph.items():
        for dep in deps:
            in_degree[task_id] += 1
    
    # Start with tasks that have no dependencies
    queue = [tid for tid in graph if in_degree[tid] == 0]
    result = []
    
    while queue:
        # Sort to ensure deterministic order
        queue.sort()
        current = queue.pop(0)
        if current in task_map:
            result.append(task_map[current])
        
        # Reduce in-degree for dependent tasks
        for task_id, deps in graph.items():
            if current in deps:
                in_degree[task_id] -= 1
                if in_degree[task_id] == 0 and task_id not in [r.id for r in result]:
                    queue.append(task_id)
    
    return result

def get_executable_tasks(tasks: List[Task]) -> List[Task]:
    """Get tasks that can be executed (dependencies satisfied, not completed)."""
    task_map = {t.id: t for t in tasks}
    executable = []
    
    for task in tasks:
        if task.completed:
            continue
        
        # Check if all dependencies are completed
        deps_satisfied = True
        for dep_id in task.depends_on:
            if dep_id in task_map and not task_map[dep_id].completed:
                deps_satisfied = False
                break
        
        if deps_satisfied:
            executable.append(task)
    
    return executable

def get_parallel_tasks(tasks: List[Task]) -> List[List[Task]]:
    """Group tasks that can run in parallel."""
    task_map = {t.id: t for t in tasks}
    remaining = [t for t in tasks if not t.completed]
    completed_ids = {t.id for t in tasks if t.completed}
    
    parallel_groups = []
    
    while remaining:
        # Find all tasks whose dependencies are satisfied
        current_group = []
        for task in remaining:
            deps_satisfied = all(
                dep_id in completed_ids or dep_id not in task_map
                for dep_id in task.depends_on
            )
            if deps_satisfied:
                current_group.append(task)
        
        if not current_group:
            # Circular dependency or error
            break
        
        parallel_groups.append(current_group)
        
        # Mark current group as "completed" for next iteration
        for task in current_group:
            completed_ids.add(task.id)
            remaining.remove(task)
    
    return parallel_groups

def _parse_cli_args(argv: List[str]) -> Dict[str, object]:
    log_level, args, log_error = split_log_level(argv)
    if log_error:
        raise ConfigError(log_error)

    output_json = '--json' in args
    show_next = '--next' in args
    show_parallel = '--parallel' in args

    tasks_file = None
    for arg in args:
        if not arg.startswith('-'):
            tasks_file = arg
            break

    if not tasks_file:
        raise ConfigError(
            "Missing tasks file path.",
            suggestion="Provide a tasks.md path as the first argument.",
            details={"usage": (__doc__ or "").strip()},
        )

    return {
        "tasks_file": tasks_file,
        "output_json": output_json,
        "show_next": show_next,
        "show_parallel": show_parallel,
        "log_level": log_level,
    }


def main():
    try:
        opts = _parse_cli_args(sys.argv[1:])
        tasks_file = opts["tasks_file"]
        output_json = opts["output_json"]
        show_next = opts["show_next"]
        show_parallel = opts["show_parallel"]
        logger = Logger("parse_tasks", Path.cwd() / ".ai" / "logs", level=opts["log_level"])

        logger.info("parse start", {"tasks_file": tasks_file})
        try:
            with open(tasks_file, 'r', encoding='utf-8') as f:
                content = f.read()
        except FileNotFoundError as exc:
            raise ConfigError(
                f"Tasks file not found: {tasks_file}",
                suggestion="Check the path and try again.",
                details={"path": tasks_file},
            ) from exc

        tasks = parse_tasks(content)

        if show_next:
            executable = get_executable_tasks(tasks)
            if output_json:
                print(json.dumps([t.to_dict() for t in executable], indent=2, ensure_ascii=True))
            else:
                if executable:
                    print("Next executable task(s):")
                    for t in executable:
                        status = "[x]" if t.completed else "[ ]"
                        deps = f" (depends on: {', '.join(t.depends_on)})" if t.depends_on else ""
                        print(f"  {status} {t.id}. {t.title}{deps}")
                else:
                    print("No executable tasks (all completed or blocked)")

        elif show_parallel:
            groups = get_parallel_tasks(tasks)
            if output_json:
                result = [[t.to_dict() for t in g] for g in groups]
                print(json.dumps(result, indent=2, ensure_ascii=True))
            else:
                print("Parallel execution groups:")
                for i, group in enumerate(groups, 1):
                    print(f"\n  Wave {i}:")
                    for t in group:
                        deps = f" (depends on: {', '.join(t.depends_on)})" if t.depends_on else ""
                        print(f"    - {t.id}. {t.title}{deps}")

        else:
            # Default: show all tasks in topological order
            sorted_tasks = topological_sort(tasks)
            if output_json:
                print(json.dumps([t.to_dict() for t in sorted_tasks], indent=2, ensure_ascii=True))
            else:
                print("Tasks (topological order):")
                for t in sorted_tasks:
                    status = "[x]" if t.completed else "[ ]"
                    deps = f" (depends on: {', '.join(t.depends_on)})" if t.depends_on else ""
                    print(f"  {status} {t.id}. {t.title}{deps}")
        logger.info("parse complete", {"tasks_file": tasks_file})
    except AWKError as err:
        print_error(err)
        sys.exit(err.exit_code)
    except Exception as exc:
        err = handle_unexpected_error(exc)
        print_error(err)
        sys.exit(err.exit_code)

if __name__ == '__main__':
    main()
