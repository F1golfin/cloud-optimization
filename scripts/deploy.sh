#!/usr/bin/env bash
# Déploie les workloads compilés sur les VM (qui doivent être démarrées).
#
# Usage : ./scripts/deploy.sh            (toutes les VM de data/machines.csv)
#         VMS="cloudopt-vm1" ./scripts/deploy.sh
#
# Principe : on compile UNE fois, sur la machine de contrôle, pour linux/amd64
# (make linux), puis on copie les exécutables. Rien à compiler ni à installer
# côté VM, à part le paquet « time » utilisé par le script de benchmark.
#
# Arborescence créée sur chaque VM :
#   ~/cloudopt/bin/<workload>          exécutables x86_64
#   ~/cloudopt/scripts/benchmark.sh    même script que le benchmark local
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/config.sh

make linux

for vm in $VMS; do
    echo "===== $vm ====="
    # Juste après create/start, une VM est RUNNING mais sshd n'accepte pas encore
    # les connexions : on attend (jusqu'à ~3 min).
    for try in $(seq 1 12); do
        gssh "$vm" --command=true >/dev/null 2>&1 && break
        [[ $try == 12 ]] && { echo "Erreur : $vm injoignable en SSH" >&2; exit 1; }
        echo "  SSH pas encore prêt, nouvel essai dans 15 s ($try/12)..."
        sleep 15
    done
    gssh "$vm" --command="mkdir -p ~/$REMOTE_DIR/bin ~/$REMOTE_DIR/scripts ~/$REMOTE_DIR/results
        command -v perl >/dev/null || { sudo apt-get update -qq && sudo apt-get install -y -qq perl; }
        [ -x /usr/bin/time ] || { sudo apt-get update -qq && sudo apt-get install -y -qq time; }"
    gscp bin/linux-amd64/* "$vm:~/$REMOTE_DIR/bin/"
    gscp scripts/benchmark.sh "$vm:~/$REMOTE_DIR/scripts/"
    # Vérification : chaque binaire s'exécute bien sur la VM.
    gssh "$vm" --command="cd ~/$REMOTE_DIR && chmod +x bin/* scripts/*.sh && uname -m &&
        for w in bin/*; do ./\$w --size 100 >/dev/null || exit 1; echo \"  ok \$w\"; done"
done

echo "Déploiement terminé sur : $VMS"
