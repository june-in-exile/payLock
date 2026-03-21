#[test_only]
module orca::paywall_tests {
    use sui::test_scenario::{Self as ts, Scenario};
    use sui::coin;
    use sui::sui::SUI;
    use sui::object;
    use std::string;

    use orca::paywall::{Self, Video, AccessPass};

    const CREATOR: address = @0xCAFE;
    const BUYER: address = @0xBEEF;
    const PRICE: u64 = 100_000_000; // 0.1 SUI

    fun create_test_video(scenario: &mut Scenario) {
        ts::next_tx(scenario, CREATOR);
        {
            let ctx = ts::ctx(scenario);
            paywall::create_video(
                PRICE,
                string::utf8(b"preview_blob_abc"),
                string::utf8(b"full_blob_xyz"),
                ctx,
            );
        };
    }

    #[test]
    fun test_create_video() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        ts::next_tx(&mut scenario, CREATOR);
        {
            let video = ts::take_shared<Video>(&scenario);
            assert!(paywall::video_price(&video) == PRICE);
            assert!(paywall::video_creator(&video) == CREATOR);
            assert!(paywall::video_preview_blob_id(&video) == &string::utf8(b"preview_blob_abc"));
            assert!(paywall::video_full_blob_id(&video) == &string::utf8(b"full_blob_xyz"));
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    fun test_purchase_exact_payment() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let payment = coin::mint_for_testing<SUI>(PRICE, ctx);
            let pass = paywall::purchase(&video, payment, ctx);

            assert!(paywall::access_pass_video_id(&pass) == object::id(&video));

            transfer::public_transfer(pass, BUYER);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    fun test_purchase_overpayment() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let payment = coin::mint_for_testing<SUI>(PRICE * 2, ctx);
            let pass = paywall::purchase(&video, payment, ctx);

            transfer::public_transfer(pass, BUYER);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    #[expected_failure(abort_code = paywall::EInsufficientPayment)]
    fun test_purchase_insufficient_payment() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let payment = coin::mint_for_testing<SUI>(PRICE - 1, ctx);
            let pass = paywall::purchase(&video, payment, ctx);

            transfer::public_transfer(pass, BUYER);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    fun test_seal_approve_valid() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        // Purchase first
        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let payment = coin::mint_for_testing<SUI>(PRICE, ctx);
            let pass = paywall::purchase(&video, payment, ctx);
            transfer::public_transfer(pass, BUYER);
            ts::return_shared(video);
        };

        // Seal approve
        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let pass = ts::take_from_sender<AccessPass>(&scenario);

            // Build a valid seal ID: video object ID bytes + extra data
            let mut seal_id = object::id_bytes(&video);
            vector::push_back(&mut seal_id, 0x01);
            vector::push_back(&mut seal_id, 0x02);

            paywall::seal_approve(seal_id, &pass, &video);

            ts::return_to_sender(&scenario, pass);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    #[expected_failure(abort_code = paywall::EVideoMismatch)]
    fun test_seal_approve_wrong_video() {
        let mut scenario = ts::begin(CREATOR);

        // Create two videos
        create_test_video(&mut scenario);

        ts::next_tx(&mut scenario, CREATOR);
        {
            let ctx = ts::ctx(&mut scenario);
            paywall::create_video(
                PRICE,
                string::utf8(b"other_preview"),
                string::utf8(b"other_full"),
                ctx,
            );
        };

        // Purchase first video
        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let payment = coin::mint_for_testing<SUI>(PRICE, ctx);
            let pass = paywall::purchase(&video, payment, ctx);
            transfer::public_transfer(pass, BUYER);
            ts::return_shared(video);
        };

        // Try seal_approve with pass from video1 but referencing video2
        ts::next_tx(&mut scenario, BUYER);
        {
            // Take the second video (different from purchased)
            let video1 = ts::take_shared<Video>(&scenario);
            let video2 = ts::take_shared<Video>(&scenario);
            let pass = ts::take_from_sender<AccessPass>(&scenario);

            let seal_id = object::id_bytes(&video2);
            // pass is for video1, but we pass video2 — should fail
            paywall::seal_approve(seal_id, &pass, &video2);

            ts::return_to_sender(&scenario, pass);
            ts::return_shared(video1);
            ts::return_shared(video2);
        };

        ts::end(scenario);
    }

    #[test]
    fun test_update_full_blob_id() {
        let mut scenario = ts::begin(CREATOR);

        // Create video with empty full_blob_id
        ts::next_tx(&mut scenario, CREATOR);
        {
            let ctx = ts::ctx(&mut scenario);
            paywall::create_video(
                PRICE,
                string::utf8(b"preview_blob_abc"),
                string::utf8(b""),
                ctx,
            );
        };

        // Creator updates full_blob_id
        ts::next_tx(&mut scenario, CREATOR);
        {
            let mut video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            paywall::update_full_blob_id(
                &mut video,
                string::utf8(b"encrypted_full_blob"),
                ctx,
            );
            assert!(paywall::video_full_blob_id(&video) == &string::utf8(b"encrypted_full_blob"));
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    #[expected_failure(abort_code = paywall::ENotCreator)]
    fun test_update_full_blob_id_not_creator() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        // Non-creator tries to update
        ts::next_tx(&mut scenario, BUYER);
        {
            let mut video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            paywall::update_full_blob_id(
                &mut video,
                string::utf8(b"hacked_blob"),
                ctx,
            );
            ts::return_shared(video);
        };

        ts::end(scenario);
    }

    #[test]
    fun test_purchase_and_transfer() {
        let mut scenario = ts::begin(CREATOR);
        create_test_video(&mut scenario);

        ts::next_tx(&mut scenario, BUYER);
        {
            let video = ts::take_shared<Video>(&scenario);
            let ctx = ts::ctx(&mut scenario);
            let payment = coin::mint_for_testing<SUI>(PRICE, ctx);
            paywall::purchase_and_transfer(&video, payment, ctx);
            ts::return_shared(video);
        };

        // Verify buyer received AccessPass
        ts::next_tx(&mut scenario, BUYER);
        {
            let pass = ts::take_from_sender<AccessPass>(&scenario);
            let video = ts::take_shared<Video>(&scenario);
            assert!(paywall::access_pass_video_id(&pass) == object::id(&video));
            ts::return_to_sender(&scenario, pass);
            ts::return_shared(video);
        };

        ts::end(scenario);
    }
}
