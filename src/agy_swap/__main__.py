"""Main entry point and CLI argument parser."""

import argparse
import sys

from agy_swap import VERSION, AccountStoreError, AmbiguousAccountError, RED, RESET
from agy_swap.commands import (
    cmd_add, cmd_list, cmd_limits, cmd_set_limit, cmd_logout,
    cmd_next, cmd_switch, cmd_remove, cmd_status,
)
from agy_swap.credentials import enable_windows_ansi
from agy_swap.display import configure_output
from agy_swap.tui import cmd_interactive
from agy_swap.updater import cmd_update


def main():
    configure_output()
    enable_windows_ansi()
    parser = argparse.ArgumentParser(description="Minimal Multi-Account Switcher for Google Antigravity CLI (agy)")
    parser.add_argument("-v", "--version", action="version", version=f"agy-swap v{VERSION}")
    parser.add_argument("--add", action="store_true", help="Log in & add a Google account to managed accounts")
    parser.add_argument("--token", choices=["-"], metavar="-", help="Import a token from stdin. Use with --add.")
    parser.add_argument("--list", action="store_true", help="List all saved accounts")
    parser.add_argument("--next", "--auto-rotate", action="store_true", help="Rotate to an account with available quota")
    parser.add_argument("--family", choices=["claude", "gemini", "gpt"], help="Filter quota by model family")
    parser.add_argument("--switch-to", dest="switch_to", metavar="ACCOUNT", help="Switch to specific account (email or number)")
    parser.add_argument("--switch", action="store_true", help="Rotate/interactive switch")
    parser.add_argument("--remove", dest="remove_account", metavar="ACCOUNT", help="Remove a saved account")
    parser.add_argument("--status", action="store_true", help="Show active account details")
    parser.add_argument("--logout", action="store_true", help="Logout of active session")

    subparsers = parser.add_subparsers(dest="command", help="Subcommand to run")
    add_parser = subparsers.add_parser("add", help="Log in & add a Google account")
    add_parser.add_argument("--token", choices=["-"], metavar="-", default=argparse.SUPPRESS, help="Import a token from stdin")
    subparsers.add_parser("list", help="List all saved accounts")
    limits_parser = subparsers.add_parser("limits", help="Show account quota limits")
    limits_parser.add_argument("--verbose", action="store_true", help="Show cooldown provenance and observation time")
    limits_parser.add_argument("--refresh", action="store_true", help="Force a fresh Google quota request")
    subparsers.add_parser("logout", help="Logout of active session")
    next_parser = subparsers.add_parser("next", help="Rotate to an account with available quota")
    next_parser.add_argument("--family", choices=["claude", "gemini", "gpt"], default=argparse.SUPPRESS)
    
    switch_parser = subparsers.add_parser("switch", help="Switch to an account")
    switch_parser.add_argument("account", nargs="?", help="Account email or index")

    remove_parser = subparsers.add_parser("remove", help="Remove a saved account")
    remove_parser.add_argument("account", nargs="?", help="Account email or index")

    subparsers.add_parser("status", help="Show active account details")

    update_parser = subparsers.add_parser("update", help="Update agy-swap to the latest version")
    update_parser.add_argument("--force", action="store_true", help="Re-install even if already up to date")

    limit_parser = subparsers.add_parser("limit", help="Manage a manual quota cooldown")
    limit_subparsers = limit_parser.add_subparsers(dest="limit_command", required=True)
    limit_set = limit_subparsers.add_parser("set", help="Set or clear a cooldown")
    limit_set.add_argument("account", help="Account email or index")
    limit_set.add_argument("duration", help="Duration such as 4h30m, 6d, or reset")
    limit_set.add_argument("--group", choices=["claude", "gemini", "gpt"], default="claude")

    args = parser.parse_args()

    try:
        if args.add or args.command == "add" or args.token is not None:
            cmd_add(args)
        elif args.list or args.command == "list":
            cmd_list(args)
        elif args.command == "limits":
            cmd_limits(args)
        elif args.command == "limit" and args.limit_command == "set":
            cmd_set_limit(args)
        elif args.logout or args.command == "logout":
            cmd_logout(args)
        elif args.next or args.command == "next":
            cmd_next(args)
        elif args.switch_to:
            args.account = args.switch_to
            cmd_switch(args)
        elif args.switch:
            args.account = None
            cmd_switch(args)
        elif args.remove_account:
            args.account = args.remove_account
            cmd_remove(args)
        elif args.command == "switch":
            cmd_switch(args)
        elif args.command == "remove":
            cmd_remove(args)
        elif args.command == "status" or args.status:
            cmd_status(args)
        elif args.command == "update":
            cmd_update(args)
        else:
            cmd_interactive(args)
    except (AccountStoreError, AmbiguousAccountError) as exc:
        print(f"{RED}Error: {exc}{RESET}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
