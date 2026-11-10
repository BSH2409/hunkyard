#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'hunkyard: %s\n' "$*" >&2
  exit 1
}

main() {
  for command in curl git uname mktemp mkdir chmod mv rm awk grep; do
    command -v "$command" >/dev/null 2>&1 || fail "$command is required. Install it and run this installer again."
  done

  case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) fail 'This installer supports Linux and macOS. On Windows, use WSL or download a Windows binary from GitHub Releases.' ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) fail 'Unsupported CPU architecture. Release binaries support x86_64 and ARM64.' ;;
  esac

  if command -v sha256sum >/dev/null 2>&1; then
    checksum_command=(sha256sum)
  elif command -v shasum >/dev/null 2>&1; then
    checksum_command=(shasum -a 256)
  else
    fail 'SHA-256 verification requires sha256sum or shasum. Install one and try again.'
  fi

  release_root=https://github.com/BSH2409/hunkyard/releases
  curl_flags=(--fail --silent --show-error --location --proto '=https' --proto-redir '=https' --retry 3 --connect-timeout 15)
  version=${HUNKYARD_VERSION:-}
  if [ -z "$version" ]; then
    latest_url=$(curl "${curl_flags[@]}" --output /dev/null --write-out '%{url_effective}' "$release_root/latest") || fail 'Could not find the latest release. Check your connection to GitHub.'
    version=${latest_url##*/}
  fi
  [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]] || fail "Invalid release version: $version"

  install_dir=${HUNKYARD_INSTALL_DIR:-${HOME:?HOME must be set}/.local/bin}
  [[ "$install_dir" = /* ]] || fail 'HUNKYARD_INSTALL_DIR must be an absolute path.'
  mkdir -p "$install_dir" || fail "Cannot create $install_dir. Choose a writable HUNKYARD_INSTALL_DIR."
  [ ! -d "$install_dir/hunkyard" ] || fail "$install_dir/hunkyard is a directory. Choose another install location."
  staging_dir=$(mktemp -d "$install_dir/.hunkyard-install.XXXXXX") || fail "Cannot write to $install_dir. Choose a writable HUNKYARD_INSTALL_DIR."
  trap 'rm -rf "$staging_dir"' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM

  asset=hunkyard-$os-$arch
  release_url=$release_root/download/$version
  printf 'Installing Hunkyard %s for %s/%s...\n' "$version" "$os" "$arch"
  curl "${curl_flags[@]}" --output "$staging_dir/hunkyard" "$release_url/$asset" || fail "Could not download $asset for $version. Check the release and your connection."
  curl "${curl_flags[@]}" --output "$staging_dir/checksums.txt" "$release_url/checksums.txt" || fail 'Could not download release checksums.'
  expected_checksum=$(awk -v asset="$asset" '$2 == asset {print $1}' "$staging_dir/checksums.txt")
  [[ "$expected_checksum" =~ ^[0-9a-f]{64}$ ]] || fail "Missing or invalid checksum for $asset. Nothing was installed."
  actual_checksum=$("${checksum_command[@]}" "$staging_dir/hunkyard")
  [ "${actual_checksum%% *}" = "$expected_checksum" ] || fail 'Checksum mismatch. Nothing was installed; try downloading again.'
  chmod 755 "$staging_dir/hunkyard"
  "$staging_dir/hunkyard" --version || fail 'The downloaded binary cannot run on this system. Nothing was installed.'
  mv -f "$staging_dir/hunkyard" "$install_dir/hunkyard"
  printf 'Installed to %s/hunkyard\n' "$install_dir"

  case ":$PATH:" in
    *":$install_dir:"*) ;;
    *)
      printf -v path_line 'export PATH=%q:"$PATH"' "$install_dir"
      profile=
      case "${SHELL:-}" in
        */zsh) profile=$HOME/.zshrc ;;
        */bash)
          if [ "$os" = darwin ]; then
            profile=$HOME/.bash_profile
            for login_file in .bash_profile .bash_login .profile; do
              if [ -f "$HOME/$login_file" ]; then profile=$HOME/$login_file; break; fi
            done
          else
            profile=$HOME/.bashrc
          fi
          ;;
      esac
      if [ -n "$profile" ]; then
        if [ ! -f "$profile" ] || ! grep -Fqx "$path_line" "$profile"; then
          printf '\n%s\n' "$path_line" >> "$profile"
        fi
        printf 'PATH configured in %s. Open a new terminal, or run:\n  %s\n' "$profile" "$path_line"
      else
        printf 'Add the install directory to your shell PATH. For Bash or Zsh, run:\n  %s\n' "$path_line"
      fi
      ;;
  esac
  printf '\nRun hunkyard in a Git repository to review changes.\n'
  printf 'Connect your agent with: hunkyard install --global codex (or claude).\n'
}

main
