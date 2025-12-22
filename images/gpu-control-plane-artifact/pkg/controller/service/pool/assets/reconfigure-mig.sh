
#!/usr/bin/env bash
# Copyright 2025 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# Based on NVIDIA MIG manager scripts:
# Copyright (c) 2021, NVIDIA CORPORATION.  All rights reserved.

WITH_REBOOT="false"
WITH_SHUTDOWN_HOST_GPU_CLIENTS="false"
HOST_ROOT_MOUNT=""
HOST_NVIDIA_DIR=""
HOST_MIG_MANAGER_STATE_FILE=""
HOST_GPU_CLIENT_SERVICES=""
HOST_KUBELET_SERVICE=""
NODE_NAME=""
MIG_CONFIG_FILE=""
SELECTED_MIG_CONFIG=""
DEFAULT_GPU_CLIENTS_NAMESPACE=""
CDI_ENABLED="false"
DRIVER_ROOT=""
DRIVER_ROOT_CTR_PATH=""
DEV_ROOT=""
DEV_ROOT_CTR_PATH=""
DRIVER_LIBRARY_PATH=""
NVIDIA_SMI_PATH=""
NVIDIA_CDI_HOOK_PATH=""

MAX_RETRIES=20

export SYSTEMD_LOG_LEVEL="info"

function usage() {
  echo "USAGE:"
  echo "    ${0} -h "
  echo "    ${0} -n <node> -f <config-file> -c <selected-config> -p <default-gpu-clients-namespace> [-e -t <driver-root> -a <driver-root-ctr-path> -b <dev-root> -j <dev-root-ctr-path> -l <driver-library-path> -q <nvidia-smi-path> -s <nvidia-cdi-hook-path> ] [ -m <host-root-mount> -i <host-nvidia-dir> -o <host-mig-manager-state-file> -g <host-gpu-client-services> -k <host-kubelet-service> -r -s ]"
  echo ""
  echo "OPTIONS:"
  echo "    -h                                            Display this help message"
  echo "    -r                                            Automatically reboot the node if changing the MIG mode fails for any reason"
  echo "    -d                                            Automatically shutdown/restart any required host GPU clients across a MIG configuration"
  echo "    -e                                            Enable CDI support"
  echo "    -n <node>                                     The kubernetes node to change the MIG configuration on"
  echo "    -f <config-file>                              The mig-parted configuration file"
  echo "    -c <selected-config>                          The selected mig-parted configuration to apply to the node"
  echo "    -m <host-root-mount>                          Container path where host root directory is mounted"
  echo "    -i <host-nvidia-dir>                          Host path of the directory where NVIDIA managed software directory is typically located"
  echo "    -o <host-mig-manager-state-file>              Host path where the systemd mig-manager state file is located"
  echo "    -g <host-gpu-client-services>                 Comma separated list of host systemd services to shutdown/restart across a MIG reconfiguration"
  echo "    -k <host-kubelet-service>                     Name of the host's 'kubelet' systemd service which may need to be shutdown/restarted across a MIG mode reconfiguration"
  echo "    -p <default-gpu-clients-namespace>            Default name of the Kubernetes Namespace in which the GPU client Pods are installed in"
  echo "    -t <driver-root>                              Root path to the NVIDIA driver installation"
  echo "    -a <driver-root-ctr-path>                     Root path to the NVIDIA driver installation mounted in the container"
  echo "    -b <dev-root>                                 Root path to the NVIDIA device nodes"
  echo "    -j <dev-root-ctr-path>                        Root path to the NVIDIA device nodes mounted in the container"
  echo "    -l <driver-library-path>                      Path to libnvidia-ml.so.1 in the container"
  echo "    -q <nvidia-smi-path>                          Path to nvidia-smi in the container"
  echo "    -s <nvidia-cdi-hook-path>                     Path to nvidia-cdi-hook on the host"
}

while getopts "hrden:f:c:m:i:o:g:k:p:t:a:b:j:l:q:s:" opt; do
  case ${opt} in
    h ) # process option h
      usage; exit 0
      ;;
    r ) # process option r
      WITH_REBOOT="true"
      ;;
    d ) # process option d
      WITH_SHUTDOWN_HOST_GPU_CLIENTS="true"
      ;;
    e) # process option e
      CDI_ENABLED="true"
      ;;
    n ) # process option n
      NODE_NAME=${OPTARG}
      ;;
    f ) # process option f
      MIG_CONFIG_FILE=${OPTARG}
      ;;
    c ) # process option c
      SELECTED_MIG_CONFIG=${OPTARG}
      ;;
    m ) # process option m
      HOST_ROOT_MOUNT=${OPTARG}
      ;;
    i ) # process option i
      HOST_NVIDIA_DIR=${OPTARG}
      ;;
    o ) # process option o
      HOST_MIG_MANAGER_STATE_FILE=${OPTARG}
      ;;
    g ) # process option g
      HOST_GPU_CLIENT_SERVICES=${OPTARG}
      ;;
    k ) # process option k
      HOST_KUBELET_SERVICE=${OPTARG}
      ;;
    p ) # process option p
      DEFAULT_GPU_CLIENTS_NAMESPACE=${OPTARG}
      ;;
    t ) # process option t
      DRIVER_ROOT=${OPTARG}
      ;;
    a ) # process option a
      DRIVER_ROOT_CTR_PATH=${OPTARG}
      ;;
    b ) # process option b
      DEV_ROOT=${OPTARG}
      ;;
    j ) # process option j
      DEV_ROOT_CTR_PATH=${OPTARG}
      ;;
    l ) # process option l
      DRIVER_LIBRARY_PATH=${OPTARG}
      ;;
    q ) # process option q
      NVIDIA_SMI_PATH=${OPTARG}
      ;;
    s ) # process option s
      NVIDIA_CDI_HOOK_PATH=${OPTARG}
      ;;
    \\? ) # process option ?
      echo "Invalid option: -${OPTARG}" 1>&2
      usage; exit 1
      ;;
  esac
done

shift $((OPTIND -1))
if [ -n "${HOST_MIG_MANAGER_STATE_FILE}" ]; then
  HOST_MIG_MANAGER_DIR=$(dirname "${HOST_MIG_MANAGER_STATE_FILE}")
fi

if [ -z "${NODE_NAME}" ]; then
  echo "Node Name is required"
  usage; exit 1
fi

if [ -z "${MIG_CONFIG_FILE}" ]; then
  echo "MIG Config file path is required"
  usage; exit 1
fi

if [ -z "${DEFAULT_GPU_CLIENTS_NAMESPACE}" ]; then
  echo "default GPU Client namespace is required"
  usage; exit 1
fi

# Try to set the path for Nvidia-smi if it doesn't exist
if [ -z "${NVIDIA_SMI_PATH}" ]; then
  if [ -f "${DRIVER_ROOT}/bin/nvidia-smi" ]; then
    NVIDIA_SMI_PATH="${DRIVER_ROOT}/bin/nvidia-smi"
  else
    NVIDIA_SMI_PATH=$(which nvidia-smi)
  fi
fi

# NVML library used by ctk/ctk-mig executes searches for a configured NVML (libnvidia-ml.so.1)
# first from $LD_LIBRARY_PATH. We export this here to allow ctk to locate it in the container
if [ -z "${DRIVER_LIBRARY_PATH}" ]; then
  DRIVER_LIBRARY_PATH=$(ldconfig -p | grep libnvidia-ml.so.1 | head -n 1 | sed -e 's@.* => @@')
fi
export LD_LIBRARY_PATH=$(dirname "$DRIVER_LIBRARY_PATH")
export NVIDIA_DRIVER_CAPABILITIES=all
export NVIDIA_VISIBLE_DEVICES=all

# Convert HOST_GPU_CLIENT_SERVICES to an array if it is set
if [ ! -z "${HOST_GPU_CLIENT_SERVICES}" ]; then
  readarray -td, HOST_GPU_CLIENT_SERVICES < <(printf '%s' "${HOST_GPU_CLIENT_SERVICES}"); declare -p HOST_GPU_CLIENT_SERVICES;
  HOST_GPU_CLIENT_SERVICES=("${HOST_GPU_CLIENT_SERVICES[@]%%[$'\\r\\n']}")
else
  HOST_GPU_CLIENT_SERVICES=("")
fi

# Initiate some variables
CURRENT_MIG_MODE=-1
EXIT_CODE=-1
HOST_MIG_MODE=-1

# Utility function to convert hex to decimal
function hex2dec() {
  printf "%d\\n" $1
}

# Utility function to convert decimal to hex
function dec2hex() {
  printf "0x%04x\\n" $1
}

# Utility function to convert decimal strings to binary
function dec2bin() {
  python3 -c 'print(format(int(input()),"b"))'
}

# Utility function to get pci bus id from uuid
# input: gpu uuid
# output: pci bus id
function pci_bus_id_from_uuid() {
    local gpu_uuid=$1
    ${NVIDIA_SMI_PATH} --query-gpu=gpu_bus_id --format=csv,noheader --id=${gpu_uuid}
}

# Utility function to get gpu count
function gpu_count() {
    ${NVIDIA_SMI_PATH} --query-gpu=count --format=csv,noheader | head -n 1
}

# Utility function to get gpu uuid by index
# input: gpu index
# output: gpu uuid
function gpu_uuid_from_index() {
    local gpu_index=$1
    ${NVIDIA_SMI_PATH} --query-gpu=uuid --format=csv,noheader --id=${gpu_index} | sed 's/^GPU-//'
}

function is_nvidia_gpu_present() {
  if ! [ -f "/proc/driver/nvidia/gpus/0000:00:00.0/registry" ] && ! [ -f "/proc/driver/nvidia-vgpu/version" ]; then
    echo "No NVIDIA GPU found on node - exiting"
    EXIT_CODE=0
    exit ${EXIT_CODE}
  fi
}

# Utility function to restart Kubelet
function restart_kubelet() {
  local kubelet_service=${HOST_KUBELET_SERVICE}
  if [ ! -z "${kubelet_service}" ]; then
    echo "Restarting ${kubelet_service} to ensure kubelet picks up new GPU configuration"
    chroot "${HOST_ROOT_MOUNT}" systemctl restart "${kubelet_service}"
    if [ $? -ne 0 ]; then
      echo "ERROR: Restart of ${kubelet_service} failed - please perform a manual restart after reviewing the following error message:"
      chroot "${HOST_ROOT_MOUNT}" systemctl status "${kubelet_service}"
    fi
  else
    echo "NOTE: No kubelet systemd service specified - not restarting kubelet"
  fi
}

# Utility function to (conditionally) reboot the node
function reboot_node() {
  if [[ "${WITH_REBOOT}" == "true" ]] && [[ "${HOST_KUBELET_SERVICE}" != "" ]]; then
    echo "Rebooting node to apply GPU configuration change"
    chroot "${HOST_ROOT_MOUNT}" reboot || true
  fi
}

# Utility function to shutdown/start any host GPU clients
function shutdown_host_gpu_clients() {
  local operation=$1
  local host_gpu_client_services=${HOST_GPU_CLIENT_SERVICES[@]}
  local default_gpu_clients_namespace=${DEFAULT_GPU_CLIENTS_NAMESPACE}
  local ns="--namespace=${default_gpu_clients_namespace}"
  if [ "${operation}" == "stop" ]; then
    if [ "${WITH_SHUTDOWN_HOST_GPU_CLIENTS}" == "true" ]; then
      if [[ -z "${host_gpu_client_services}" && -z "${default_gpu_clients_namespace}" ]]; then
        echo "WARN: set WITH_SHUTDOWN_HOST_GPU_CLIENTS=true, but neither HOST_GPU_CLIENT_SERVICES nor DEFAULT_GPU_CLIENTS_NAMESPACE is provided, ignoring"
        return
      fi
      echo "Stopping host GPU clients ..."
    else
      return
    fi
  elif [ "${operation}" == "start" ]; then
    echo "Starting host GPU clients ..."
  fi

  for svc in ${host_gpu_client_services}; do
    echo "  ${operation} host GPU systemd service: ${svc}"
    chroot "${HOST_ROOT_MOUNT}" systemctl ${operation} ${svc}
  done

  if [ ! -z "${default_gpu_clients_namespace}" ]; then
    clients=$(chroot "${HOST_ROOT_MOUNT}" kubectl ${ns} get pod -o=custom-columns=STATUS:.status.phase,NAME:.metadata.name -l app=nvidia-dcgm | grep -v STATUS) || true
    if [ ! -z "${clients}" ]; then
      for client in ${clients}; do
        pod=$(echo "${client}" | cut -d" " -f2)
        echo "  ${operation} DCGM client Pod: ${pod}"
        chroot "${HOST_ROOT_MOUNT}" kubectl ${ns} ${operation} pod "${pod}"
      done
    fi
    cdi_exporters=$(chroot "${HOST_ROOT_MOUNT}" kubectl ${ns} get pod -o=custom-columns=STATUS:.status.phase,NAME:.metadata.name -l app=nvidia-cdi-exporter | grep -v STATUS) || true
    if [ ! -z "${cdi_exporters}" ]; then
      for client in ${cdi_exporters}; do
        pod=$(echo "${client}" | cut -d" " -f2)
        echo "  ${operation} cdi-exporter Pod: ${pod}"
        chroot "${HOST_ROOT_MOUNT}" kubectl ${ns} ${operation} pod "${pod}"
      done
    fi
  fi
}

# Get a binary representation of the current MIG mode from the state file
function get_mig_mode_from_state_file() {
  local state_file="$1"
  local curr_mig_mode=$(jq -r '."nvidia-mig-manager" | split("=") | .[1]' ${state_file})
  curr_mig_mode=$(hex2dec ${curr_mig_mode})
  if [ "${curr_mig_mode}" -eq 0 ]; then
    # MIG mode disabled
    CURRENT_MIG_MODE=0
  elif [ "${curr_mig_mode}" -eq 5 ]; then
    # MIG mode enabled
    CURRENT_MIG_MODE=1
  else
    CURRENT_MIG_MODE=-1
  fi
}

# Get current MIG mode from GPU
function get_mig_mode_from_gpu() {
  local node=$1
  local current_mig_mode=$(${NVIDIA_SMI_PATH} --query-gpu=mig.mode.current --format=csv,noheader | head -n 1)
  if [ "${current_mig_mode}" == "Disabled" ]; then
    CURRENT_MIG_MODE=0
  elif [ "${current_mig_mode}" == "Enabled" ]; then
    CURRENT_MIG_MODE=1
  else
    CURRENT_MIG_MODE=-1
  fi
  HOST_MIG_MODE=${CURRENT_MIG_MODE}
  echo "${current_mig_mode}"
}

# Get pending MIG mode from GPU
function get_pending_mig_mode_from_gpu() {
  local node=$1
  ${NVIDIA_SMI_PATH} --query-gpu=mig.mode.pending --format=csv,noheader | head -n 1
}

# Get pending MIG mode from state file
function get_pending_mig_mode_from_state_file() {
  local state_file="$1"
  local pending_mig_mode=$(jq -r '."nvidia-mig-manager" | split("=") | .[0] | split(":") | .[1]' ${state_file})
  pending_mig_mode=$(hex2dec ${pending_mig_mode})
  if [ "${pending_mig_mode}" -eq 2 ]; then
    # MIG mode disabled
    echo "Disabled"
  elif [ "${pending_mig_mode}" -eq 4 ]; then
    # MIG mode enabled
    echo "Enabled"
  else
    echo "ERROR: unknown pending MIG mode - ignoring for now"
  fi
}

# Verify MIG configuration file is valid
function verify_mig_config_file() {
  local config_file=$1
  if [[ ! -e "${config_file}" ]]; then
    echo "ERROR: MIG Config file ${config_file} does not exist"
    return 1
  fi
}

# Verify MIG device name and count
function verify_mig_device() {
  local profile=$1
  local count=$2
  if [[ ${profile} != *"g"* || ${profile} != *"gb"* ]]; then
    echo "ERROR: MIG profile ${profile} is invalid, it must specify 'g' and 'gb'"
    return 1
  fi
  if [[ ! ${profile} =~ ^([1-9][0-9]*g\\.[0-9]+gb|(1-9)[0-9]*g\\.[0-9]+gb\\.me)$ ]]; then
    echo "ERROR: MIG profile ${profile} is invalid, expected format: ^[1-9][0-9]*g\\.[0-9]+gb(\\.me)?$"
    return 1
  fi
  if [[ ! ${count} =~ ^[1-9][0-9]*$ ]]; then
    echo "ERROR: MIG device count ${count} for profile ${profile} is invalid, must be a positive integer"
    return 1
  fi
}

# Verify mig-parted config YAML is valid
function verify_mig_config_yaml() {
  local config_file=$1
  local selected_config=$2
  local devices=$3
  verify_mig_config_file ${config_file}
  if [[ $? -ne 0 ]]; then
    return 1
  fi

  local configs=$(yq eval '.["mig-configs"]' ${config_file})
  if [[ -z "${configs}" || "${configs}" == "null" ]]; then
    echo "ERROR: No mig-configs present in mig config file ${config_file}"
    return 1
  fi

  local config=$(yq -p yaml eval '.["mig-configs"] | map(select(.name == "'"${selected_config}"'")) | .[]' ${config_file})
  if [[ -z "${config}" || "${config}" == "null" ]]; then
    echo "ERROR: No mig-configs with name ${selected_config} found in mig config file ${config_file}"
    return 1
  fi

  local profiles=$(yq -p yaml eval ".devices[] | .\"mig-devices\"" - <<< "${config}")
  local profile_count=$(echo "${profiles}" | yq -p yaml eval ". | length" -)
  if [[ -z "${profile_count}" || "${profile_count}" == "0" ]]; then
    echo "ERROR: No MIG profiles found in mig config ${selected_config}"
    return 1
  fi

  # Verify profiles for each device
  for device in $(seq 0 $((${devices} - 1))); do
    local device_profiles=$(yq -p yaml eval ".devices | map(select(.\"device-filter\" == [\"device:${device}\"])) | .[] | .\"mig-devices\"" - <<< "${config}")
    if [[ -z "${device_profiles}" ]]; then
      # Use "all" block if device-specific block is missing
      device_profiles=$(yq -p yaml eval ".devices | map(select(.\"device-filter\" == \"all\" or .\"device-filter\" == [\"all\"])) | .[] | .\"mig-devices\"" - <<< "${config}")
    fi

    if [[ -z "${device_profiles}" ]]; then
      echo "ERROR: No MIG profiles found for device index ${device} in mig config ${selected_config}"
      return 1
    fi

    local total_profiles=0
    for profile in $(echo ${device_profiles} | yq -p yaml eval 'keys | .[]' -); do
      local count=$(echo ${device_profiles} | yq -p yaml eval ".\"${profile}\"" -)
      verify_mig_device ${profile} ${count}
      if [[ $? -ne 0 ]]; then
        return 1
      fi

      # Ensure at least one MIG device is specified for each profile
      if [ ${count} -lt 1 ]; then
        echo "ERROR: MIG profile ${profile} on device ${device} must have at least one instance"
        return 1
      fi
      total_profiles=$((total_profiles + count))
    done

    if [[ ${total_profiles} -lt 1 ]]; then
      echo "ERROR: The number of MIG devices on device ${device} must be at least 1"
      return 1
    fi
  done
}

# Use ctk-mig to switch MIG modes and configure partitions
function run_mig_parted() {
  local node=$1
  local config_name=$2
  local devices=$3
  local pending_mig_mode=$(get_pending_mig_mode_from_gpu ${node})
  local current_mig_mode=$(get_mig_mode_from_gpu ${node})

  if [[ "${pending_mig_mode}" == "Enabled" || "${pending_mig_mode}" == "Disabled" ]]; then
    echo "INFO: Node ${node} has pending MIG mode change: ${pending_mig_mode}"
    EXIT_CODE=3
    return
  fi

  if [[ "${current_mig_mode}" == "Disabled" ]]; then
    HOST_MIG_MODE=0
  elif [[ "${current_mig_mode}" == "Enabled" ]]; then
    HOST_MIG_MODE=1
  fi

  echo "INFO: Configuring MIG for node ${node}"
  echo "INFO: Selecting GPU topology ${config_name} from ${MIG_CONFIG_FILE}"
  echo "INFO: GPUs present on node: ${devices}"

  # Verify the MIG configuration before applying
  verify_mig_config_yaml ${MIG_CONFIG_FILE} ${config_name} ${devices}
  if [[ $? -ne 0 ]]; then
    EXIT_CODE=1
    return
  fi

  echo "INFO: Reconfiguring MIG to ${config_name} on node: ${node}"

  local args="apply --force -f ${MIG_CONFIG_FILE} --mode switch -c ${config_name} --device-filter all"

  if [[ "${WITH_REBOOT}" == "true" ]]; then
    args="${args} --reboot"
  fi
  if [[ "${WITH_SHUTDOWN_HOST_GPU_CLIENTS}" == "true" ]]; then
    args="${args} --shutdown-host-gpu-clients"
  fi
  if [[ "${CDI_ENABLED}" == "true" ]]; then
    args="${args} --cdi-root ${HOST_ROOT_MOUNT}/etc/cdi"
  fi
  # docker/tini have issues when/if nvml tries to cgroupAccessCheck()
  # cgroup utilities are only needed when ctk installs nvliblist.conf,
  # and K8s is not expected to use nvliblist.conf. Therefore ignore errors.
  args="${args} || true"

  # Execute CTK MIG Apply
  ctk mig ${args}
  EXIT_CODE=$?
}

function main() {
  # Ensure Nvidia GPU present
  is_nvidia_gpu_present
  local DEVICE_COUNT=$(gpu_count)

  # Determine current MIG mode
  CURRENT_MIG_MODE=$(get_mig_mode_from_gpu ${NODE_NAME})

  # Run shutdown_host_gpu_clients and restart kubelet only when host is doing MIG reconfiguration
  if [[ "${HOST_MIG_MODE}" == 0 ]]; then
    shutdown_host_gpu_clients stop
  fi

  run_mig_parted ${NODE_NAME} ${SELECTED_MIG_CONFIG} ${DEVICE_COUNT}

  # Restart kubelet and host gpu clients if MIG mode changes
  if [[ "${HOST_MIG_MODE}" != "${CURRENT_MIG_MODE}" ]]; then
    restart_kubelet
    shutdown_host_gpu_clients start
  fi

  # Reboot node (if requested)
  reboot_node
}

main "$@"
exit ${EXIT_CODE}
