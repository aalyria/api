# Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
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

# A simple example enactment backend written in Python.
import json
import sys
import enum

from typing import Any


NODE_ID = "esa-5g6g-hub"


class StatusCode(enum.Enum):
    """An enum mirroring the gRPC status codes.

    https://grpc.github.io/grpc/core/md_doc_statuscodes.html
    """

    OK = 0
    CANCELLED = 1
    UNKNOWN = 2
    INVALID_ARGUMENT = 3
    DEADLINE_EXCEEDED = 4
    NOT_FOUND = 5
    ALREADY_EXISTS = 6
    PERMISSION_DENIED = 7
    RESOURCE_EXHAUSTED = 8
    FAILED_PRECONDITION = 9
    ABORTED = 10
    OUT_OF_RANGE = 11
    UNIMPLEMENTED = 12
    INTERNAL = 13
    UNAVAILABLE = 14
    DATA_LOSS = 15
    UNAUTHENTICATED = 16


class StatusCodeException(Exception):
    def __init__(self, code: StatusCode, msg: str = ""):
        super().__init__(msg)
        self.code = code


def check_preconditions(req: dict[str, Any]):
    requested_node_id = req.get("nodeId")
    if requested_node_id != NODE_ID:
        raise StatusCodeException(
            StatusCode.INVALID_ARGUMENT, f"unknown node {requested_node_id}"
        )

    flow_update = req.get("change", {}).get("flowUpdate", {})
    if not flow_update:
        raise StatusCodeException(
            StatusCode.UNIMPLEMENTED,
            "this agent only handles FlowUpdate change requests",
        )

    op = flow_update.get("operation", "UNKNOWN")
    if op not in ["ADD", "DELETE"]:
        raise StatusCodeException(
            StatusCode.INVALID_ARGUMENT, f"unknown operation {op}"
        )

    rule = flow_update.get("rule")
    if not rule.get("classifier"):
        raise StatusCodeException(
            StatusCode.INVALID_ARGUMENT, "no packet classifier provided"
        )

    if not all(
        [
            act.get("actionType", {}).get("forward")
            for bucket in rule["action_bucket"]
            for act in bucket["action"]
        ]
    ):
        raise StatusCodeException(
            StatusCode.UNIMPLEMENTED,
            "this agent only handles 'forward' actions",
        )


def process(req: dict[str, Any]):
    check_preconditions(req)

    flow_update = req["change"]["flowUpdate"]
    rule_id = flow_update["flowRuleId"]
    rule = flow_update["rule"]
    is_add = rule["operation"] == "ADD"
    packet_classifier = rule["classifier"]
    fn = add_forwarding_rule if is_add else delete_forwarding_rule

    for bucket in rule["actionBucket"]:
        for action in bucket["action"]:
            out_iface_id = action["actionType"]["forward"]["outInterfaceId"]
            fn(id=rule_id, classifier=packet_classifier, out_iface=out_iface_id)


def add_forwarding_rule(id: str, classifier: dict[str, Any], out_iface: str):
    """The logic to add a forwarding rule goes here."""
    pass


def delete_forwarding_rule(id: str, classifier: dict[str, Any], out_iface: str):
    """The logic to delete a forwarding rule goes here."""
    pass


def main():
    try:
        process(json.load(sys.stdin))
        # (optional) write the new JSON-encoded ControlPlaneState to stdout
    except StatusCodeException as sce:
        sys.stderr.write(str(sce))
        sys.exit(sce.code.value)


if __name__ == "__main__":
    main()
