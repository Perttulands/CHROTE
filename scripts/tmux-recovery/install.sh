#!/usr/bin/env bash
set -euo pipefail

prefix="/usr/local"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --prefix)
      if [ "$#" -lt 2 ]; then
        echo "--prefix requires a value" >&2
        exit 2
      fi
      prefix="$2"
      shift 2
      ;;
    -h|--help)
      echo "usage: install.sh [--prefix PREFIX]"
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
lib_dir="$prefix/lib/chrote/tmux-recovery"
doc_dir="$prefix/share/doc/chrote/tmux-recovery"
bin_dir="$prefix/bin"

install -d -m 0755 "$lib_dir" "$doc_dir" "$bin_dir"

install -m 0644 \
  "$script_dir/client.py" \
  "$script_dir/collector.py" \
  "$script_dir/manifest.py" \
  "$script_dir/owner_probe.py" \
  "$script_dir/snapshot.py" \
  "$script_dir/restore.py" \
  "$script_dir/verify.py" \
  "$lib_dir/"

install -m 0644 "$script_dir/README.md" "$doc_dir/README.md"
install -m 0644 "$script_dir/recovery-manifest.schema.json" "$doc_dir/recovery-manifest.schema.json"

make_wrapper() {
  local name="$1"
  local module="$2"
  local target="$bin_dir/$name"
  {
    printf '%s\n' '#!/usr/bin/env bash'
    printf '%s\n' 'set -euo pipefail'
    printf 'exec python3 %q "$@"\n' "$lib_dir/$module"
  } > "$target"
  chmod 0755 "$target"
}

make_wrapper chrote-tmux-recovery-snapshot snapshot.py
make_wrapper chrote-tmux-recovery-restore restore.py
make_wrapper chrote-tmux-recovery-verify verify.py
make_wrapper chrote-tmux-recovery-collector collector.py

echo "installed CHROTE tmux recovery tools under $prefix"
