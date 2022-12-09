function Start-Fetch {
    $args, $ref = $Args
    if ([string]::IsNullOrEmpty($args) && [string]::IsNullOrEmpty($ref)) {
        Write-Host "+ git fetch origin"
        iu git fetch origin
    } elseif ([string]::IsNullOrEmpty($args)) {
        Write-Host "+ git fetch origin ${ref}:"
        iu git fetch origin "${ref}:"
    } else {
        Write-Host "+ git fetch ${args} origin ${ref}:"
        iu git fetch ${args} origin "${ref}:"
    }
}