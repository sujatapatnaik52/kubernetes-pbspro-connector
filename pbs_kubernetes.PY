# coding: utf-8

# Copyright (C) 1994-2019 Altair Engineering, Inc.
# For more information, contact Altair at www.altair.com.
#
# This file is part of the PBS Professional ("PBS Pro") software.
#
# Open Source License Information:
#
# PBS Pro is free software. You can redistribute it and/or modify it under the
# terms of the GNU Affero General Public License as published by the Free
# Software Foundation, either version 3 of the License, or (at your option) any
# later version.
#
# PBS Pro is distributed in the hope that it will be useful, but WITHOUT ANY
# WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS
# FOR A PARTICULAR PURPOSE.
# See the GNU Affero General Public License for more details.
#
# You should have received a copy of the GNU Affero General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.
#
# Commercial License Information:
#
# For a copy of the commercial license terms and conditions,
# go to: (http://www.pbspro.com/UserArea/agreement.html)
# or contact the Altair Legal Department.
#
# Altair’s dual-license business model allows companies, individuals, and
# organizations to create proprietary derivative works of PBS Pro and
# distribute them - whether embedded or bundled with other software -
# under a commercial license agreement.
#
# Use of Altair’s trademarks, including but not limited to "PBS™",
# "PBS Professional®", and "PBS Pro™" and Altair’s logos is subject to Altair's
# trademark licensing policies.

"""
PBS Professional hook for creation of kubernetes pod to maintain consistency of
resources used and resources available for both PBS and Kubernetes running on
the same cluster
This hook services the following events:
- execjob_end
- execjob_begin
"""

import pbs
import os
import subprocess
import traceback
import string
import sys
import json as js
import re

e = pbs.event()
euser = e.job.euser
user_home = os.path.join(os.sep, "home", euser)
hostname = pbs.get_local_nodename()

# Set values to the following
NON_DEFAULT_NAMESPACE = "default"
schedulerName = ""
serviceAccountName = ""
kubeconfig_file = os.path.join(os.sep, "root", ".kube", "config")
kubectl_path = os.path.join(os.sep, "usr", "bin", "kubectl")
dynamic_pod_path = os.path.join(os.sep, "home", euser)


def caller_name():
    """
    Returns the name of the calling function or method.
    """
    return str(sys._getframe(1).f_code.co_name)

def decode_dict(data):
    """
    Method to convert dictionary from unicode to utf-8
    """
    returnvalue = {}
    for key, value in data.items():
        if isinstance(key, bytes):
            key = key.decode('utf-8')
        if isinstance(value, bytes):
            value = value.decode('utf-8')
        elif isinstance(value, list):
            value = decode_list(value)
        elif isinstance(value, dict):
            value = decode_dict(value)
        returnvalue[key] = value
    return returnvalue

def parse_config_file():
    """
    Read in the config file and set the necessary parameters
    """
    pbs.logmsg(pbs.EVENT_DEBUG4, "%s: Method called " %
               (caller_name()))
    cfg_file = pbs.hook_config_filename
    pbs.logmsg(pbs.EVENT_DEBUG4, "%s: Config file is %s" %
               (caller_name(), cfg_file))
    try:
        cfg = js.load(open(cfg_file, 'r'), object_hook=decode_dict)
    except IOError:
        raise ConfigError("I/O error in reading config file")
    except ValueError:
        raise ConfigError("JSON parsing error in reading config file")
    except Exception:
        raise

    # Set some defaults if they are not present
    if 'kubelet_config' not in cfg.keys():
        msg = "Kubelet config path not found"

    return cfg

conf = parse_config_file()

def execjob_end_handler():
    """
    Method for pod deletion after job completion
    """
    pbs.logmsg(pbs.EVENT_DEBUG4, "%s: Method called" % (caller_name()))
    j = e.job
    if "PODNAME" in str(e.job.Variable_List):
        pbs.logmsg(pbs.LOG_DEBUG, "Nothing to do")
        return
    pbs.logmsg(pbs.LOG_DEBUG,
               "Deleting the Pod associated with job %s" % j.id)
    try:
        pod_path = os.path.join(dynamic_pod_path, 
                                str(e.job.Job_Name) + "-" + hostname + ".yaml")
        os.remove(pod_path)
    except OSError:
        pbs.logmsg(pbs.LOG_DEBUG, "Pod deletion Failed")
    podname = str(j.id) + "-" + hostname
        
    del_cmd = [kubectl_path, "--kubeconfig=" + kubeconfig_file,
               "delete", "pod", podname, "-n", NON_DEFAULT_NAMESPACE]
    pbs.logmsg(pbs.LOG_DEBUG, "Running command : %s" % ' '.join(del_cmd))
    try:
        p = subprocess.Popen(del_cmd, shell=False,
                             stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE)
        stdout, stderr = p.communicate()
    except OSError:
        pbs.logmsg(pbs.EVENT_DEBUG, "Failed to execute: %s" %
                   ' '.join(del_cmd))
    status = p.returncode
    if status != 0:
        pbs.logmsg(pbs.EVENT_DEBUG,
                   "Unable to run command: %s.\n err: %s" %
                   (' '.join(del_cmd), stderr))

def execjob_begin_handler():
    """
    Method for pod creation
    """
    pbs.logmsg(pbs.EVENT_DEBUG, "%s: Method called" % (caller_name()))
    mem = "0"
    cpu = 0
    execv_chunks = re.findall(r'\(.*?\)', str(e.job.exec_vnode))
    chunks = []

    for chunk in execv_chunks:
        chunks.extend(chunk.split("+"))

    for each in chunks:
        if hostname in each and "ncpus" in each:
            chunk_val = each.strip("(").strip(")").split(":")
            for rsc in chunk_val:
                if "ncpus" in rsc:
                    cpu += int(rsc.split("=")[1])

    if e.job.Resource_List["mem"] != None:
        mem = str(e.job.Resource_List["mem"])
        mem = mem[:-2]
    pod_path = os.path.join(dynamic_pod_path,
                            str(e.job.Job_Name) + "-" + hostname + ".yaml")
    pod_discription = """apiVersion: v1
kind: Pod
metadata:
  name: """ + str(e.job.id) + "-" + hostname + """
  namespace: """ + NON_DEFAULT_NAMESPACE + """
spec:
  schedulerName: """ + schedulerName + """
  serviceAccountName: """ + serviceAccountName + """
  nodeName: """ + hostname + """
  containers:
  - name: sleep-forever
    image: gcr.io/google_containers/pause:0.8.0
    resources:
      requests:
        cpu: """ + str(cpu) + """
        memory: """ + mem + """Mi"""
    if "PODNAME" not in str(e.job.Variable_List):
        try:
            with open(pod_path, "w") as f:
                f.write(pod_discription)
            f.close()
        except IOError:
            pbs.logmsg(pbs.EVENT_DEBUG,
                       "I/O error in writing to file: %s" % pod_path)
        cmd = [kubectl_path, "--kubeconfig=" + kubeconfig_file, "apply", "-f", pod_path]
        try:
            p = subprocess.Popen(cmd, shell=False,
                                 stdout=subprocess.PIPE,
                                 stderr=subprocess.PIPE)
            stdout, stderr = p.communicate()
        except OSError:
            pbs.logmsg(pbs.EVENT_DEBUG, "Failed to execute: %s" %
                       ' '.join(cmd))
        status = p.returncode
        if status != 0:
            pbs.logmsg(pbs.EVENT_DEBUG,
                       "Unable to run command: %s.\n err: %s" %
                       (' '.join(cmd), stderr))
        

def main():

    if e.type == pbs.EXECJOB_BEGIN:
        execjob_begin_handler()

    if e.type == pbs.EXECJOB_END:
        execjob_end_handler()

try:
    main()
except Exception as exc:
    # Catch all other exceptions and report them.
    pbs.logmsg(pbs.EVENT_DEBUG, str(
        traceback.format_exc().strip().splitlines()))
    msg = ("Unexpected error in %s handling %s event" % (e.hook_name, e.type))
    msg += (": %s %s" % (exc.__class__.__name__, str(exc.args)))
    pbs.logmsg(pbs.EVENT_ERROR, msg)
    e.reject(msg)
