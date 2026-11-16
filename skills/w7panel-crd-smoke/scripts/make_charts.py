#!/usr/bin/env python3
"""Build two isolated Helm charts for the W7Panel CRD browser smoke test."""

import argparse
import io
import re
import tarfile
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("run_id", help="6-12 lowercase letters or digits")
    parser.add_argument("--out", type=Path, default=Path("/tmp"))
    args = parser.parse_args()
    if not re.fullmatch(r"[a-z0-9]{6,12}", args.run_id):
        parser.error("run_id must contain 6-12 lowercase letters or digits")

    name = f"crd-smoke-{args.run_id}"
    group = f"crd-smoke-{args.run_id}.w7panel.test"
    crd_name = f"smokerecords.{group}"
    args.out.mkdir(parents=True, exist_ok=True)

    for version, revision_note in (("0.1.0", ""), ("0.2.0", "              revisionNote:\n                type: string\n")):
        path = args.out / f"{name}-{version}.tgz"
        if path.exists():
            parser.error(f"refusing to overwrite {path}; choose a new run_id")
        chart = f"apiVersion: v2\nname: {name}\nversion: {version}\ntype: application\n"
        crd = (
            "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\n"
            f"metadata:\n  name: {crd_name}\nspec:\n  group: {group}\n"
            "  scope: Namespaced\n  names:\n    plural: smokerecords\n"
            "    singular: smokerecord\n    kind: SmokeRecord\n"
            "  versions:\n  - name: v1\n    served: true\n    storage: true\n"
            "    schema:\n      openAPIV3Schema:\n        type: object\n"
            "        properties:\n          spec:\n            type: object\n"
            "            properties:\n              message:\n                type: string\n"
            f"{revision_note}"
        )
        configmap = (
            "apiVersion: v1\nkind: ConfigMap\nmetadata:\n"
            "  name: {{ .Release.Name }}-smoke\n"
            "data:\n  purpose: w7panel-crd-smoke\n"
        )
        with tarfile.open(path, "w:gz") as archive:
            for filename, content in (
                ("Chart.yaml", chart),
                ("crds/smokerecords.yaml", crd),
                ("templates/configmap.yaml", configmap),
            ):
                data = content.encode()
                entry = tarfile.TarInfo(f"{name}/{filename}")
                entry.size = len(data)
                entry.mtime = 0
                archive.addfile(entry, io.BytesIO(data))
        print(f"{version}: {path}")

    print(f"CRD: {crd_name}")
    print(f"Suggested release: {name}-test")


if __name__ == "__main__":
    main()
