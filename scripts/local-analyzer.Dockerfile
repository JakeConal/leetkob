# Reuse the existing checker toolchain and analyzer; only the fixed entry point
# differs. The sandbox controller supplies its full isolation profile.
FROM leetkob-local-checker:pa3
ENTRYPOINT []
