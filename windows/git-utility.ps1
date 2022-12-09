function Start-Fetch {
    $args, $ref = $Args
    if ([string]::IsNullOrEmpty($args)) {
        Write-Host "+ git fetch origin ${ref}:"
        iu git fetch origin "${ref}:"
    } else {
        Write-Host "+ git fetch ${args} origin ${ref}:"
        iu git fetch ${args} origin "${ref}:"
    }
}