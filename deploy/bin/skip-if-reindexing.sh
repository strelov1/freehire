#!/bin/sh
# ExecCondition for freehire-reindex-companies.service.
#
# Meilisearch runs ONE serial task queue, so a companies rebuild started while the jobs facet
# rebuild (freehire-reindexw) is mid-flight just queues behind it — it reads as a hang, and the
# two swaps transiently hold ~2x the index on disk. Exiting non-zero here marks this cycle
# SKIPPED (not failed), so the next daily firing is unaffected and nobody has to remember to
# re-enable a timer they stopped by hand.
#
# Test ActiveState, NOT "systemctl is-active": reindexw is Type=oneshot, and a oneshot reports
# "activating" for the WHOLE of its run and never reaches "active", so is-active returns
# non-zero throughout and this guard would never fire.
case "$(systemctl show -p ActiveState --value freehire-reindexw.service)" in
	active|activating|reloading|deactivating) exit 1 ;;
esac
exit 0
