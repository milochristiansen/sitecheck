function meta()
    return {
        name        = "Failing Check",
        description = "This check always fails (non-existent domain)",
    }
end

function check()
    local r = http_fetch("https://this-domain-definitely-does-not-exist-12345.com", {
        timeout = 5,
    })

    r.FailReason = r.Error or "request failed"
    return r
end
